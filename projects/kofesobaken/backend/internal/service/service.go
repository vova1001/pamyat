// Package service — бизнес-логика сайта и кабинета. Не знает про HTTP; к БД ходит через store.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/store"
)

// Предоплата брони столика — решение заказчика: 500 ₽, засчитывается в счёт.
const BookingPrepayKop int64 = 500_00

// Сколько держим места на мероприятии, пока гость оплачивает.
const SignupHold = 20 * time.Minute

var (
	ErrNotFound     = errors.New("не найдено")
	ErrUnauthorized = errors.New("нужен вход")
)

// ValidationError — ошибки полей формы понятным языком.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "проверьте поля формы" }

// ConflictError — действие сейчас невозможно (мест нет, бронь уже отменена и т. п.).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

type fields map[string]string

func (f fields) add(k, msg string) {
	if _, ok := f[k]; !ok {
		f[k] = msg
	}
}

func (f fields) err() error {
	if len(f) == 0 {
		return nil
	}
	return &ValidationError{Fields: f}
}

type Service struct {
	db        *pgxpool.Pool
	q         *store.Queries
	pay       payments.Provider
	channels  []notify.Channel
	loc       *time.Location
	now       func() time.Time
	publicURL string // адрес сайта — ссылки в письмах и сообщениях
	// adminPassTemp — ADMIN_PASSWORD временный (сгенерирован хостингом): при первом входе — смена.
	adminPassTemp bool
	mailDirect    bool         // письма гостям уходят с сайта (у Resend подтверждён домен)
	tgMu          sync.Mutex   // один getUpdates за раз
	tgPolling     atomic.Bool  // идёт фоновый опрос бота
	botName       atomic.Value // имя бота Telegram (string)
}

func New(db *pgxpool.Pool, pay payments.Provider, channels []notify.Channel, loc *time.Location) *Service {
	return &Service{db: db, q: store.New(db), pay: pay, channels: channels, loc: loc, now: time.Now}
}

// SetAdminPasswordTemporary — пароль администратора из окружения считать временным (см. Bootstrap).
func (s *Service) SetAdminPasswordTemporary(v bool) { s.adminPassTemp = v }

// SetPublicURL — адрес сайта для ссылок в уведомлениях (кабинет, восстановление пароля).
func (s *Service) SetPublicURL(u string) { s.publicURL = strings.TrimRight(u, "/") }

// SetClock — для тестов.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

func (s *Service) inTx(ctx context.Context, fn func(q *store.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// today — сегодняшняя дата по часам кофейни (полночь в UTC для поля DATE).
func (s *Service) today() time.Time {
	n := s.now().In(s.loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// ---------- проверка и форматирование ввода ----------

var nonDigits = regexp.MustCompile(`\D`)

// NormalizePhone приводит российский номер к виду +7 (900) 000-00-00.
func NormalizePhone(in string) (string, bool) {
	d := nonDigits.ReplaceAllString(in, "")
	if len(d) == 11 && d[0] == '8' {
		d = "7" + d[1:]
	}
	if len(d) == 10 && d[0] == '9' {
		d = "7" + d
	}
	if len(d) != 11 || d[0] != '7' {
		return "", false
	}
	return fmt.Sprintf("+7 (%s) %s-%s-%s", d[1:4], d[4:7], d[7:9], d[9:11]), true
}

var tgNick = regexp.MustCompile(`^@[A-Za-z0-9_]{3,32}$`)

// NormalizeTelegram приводит ник к виду @nick.
func NormalizeTelegram(in string) (string, bool) {
	v := strings.TrimSpace(in)
	v = strings.TrimPrefix(v, "https://t.me/")
	if !strings.HasPrefix(v, "@") {
		v = "@" + v
	}
	return v, tgNick.MatchString(v)
}

func cleanText(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\x00", "")) }

func checkLen(f fields, key, val string, min, max int, msg string) {
	n := utf8.RuneCountInString(val)
	if n < min || n > max {
		f.add(key, msg)
	}
}

// ParseClock разбирает HH:MM.
func ParseClock(v string) (pgtype.Time, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(v))
	if err != nil {
		return pgtype.Time{}, false
	}
	us := int64(t.Hour())*3600e6 + int64(t.Minute())*60e6
	return pgtype.Time{Microseconds: us, Valid: true}, true
}

// Clock форматирует время как HH:MM.
func Clock(t pgtype.Time) string {
	m := t.Microseconds / 60e6
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func clockMinutes(t pgtype.Time) int { return int(t.Microseconds / 60e6) }

// ParseDate разбирает YYYY-MM-DD.
func ParseDate(v string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	return t, err == nil
}

func Date(t time.Time) string { return t.Format("2006-01-02") }

// ---------- человеческие даты и числа для уведомлений ----------

var monthsGen = []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
var weekdaysShort = []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}

// HumanDate — «Сб, 17 октября».
func HumanDate(d time.Time) string {
	return fmt.Sprintf("%s, %d %s", weekdaysShort[d.Weekday()], d.Day(), monthsGen[d.Month()-1])
}

// Plural выбирает форму: 1 гость, 2 гостя, 5 гостей.
func Plural(n int, one, few, many string) string {
	n10, n100 := n%10, n%100
	switch {
	case n10 == 1 && n100 != 11:
		return one
	case n10 >= 2 && n10 <= 4 && (n100 < 12 || n100 > 14):
		return few
	default:
		return many
	}
}

// Rub — «1 250 ₽».
func Rub(kop int64) string {
	r := kop / 100
	s := fmt.Sprintf("%d", r)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(c)
	}
	return b.String() + " ₽"
}

func logErr(msg string, err error, args ...any) {
	slog.Error(msg, append([]any{"err", err}, args...)...)
}
