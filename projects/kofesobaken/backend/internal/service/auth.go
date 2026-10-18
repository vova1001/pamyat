package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/store"
)

const (
	SessionTTL = 30 * 24 * time.Hour
	// SetupTTL — сколько действует ссылка первого входа администратора.
	SetupTTL = 72 * time.Hour
	// ResetTTL — сколько действует ссылка восстановления пароля из письма.
	ResetTTL = time.Hour

	RoleAdmin = "admin"
	RoleStaff = "staff"
)

var (
	ErrBadLogin = errors.New("неверная почта или пароль")
	// ErrForbidden — вошёл, но раздел не для этой роли.
	ErrForbidden = errors.New("нет доступа")
	// ErrMustChangePassword — вошёл с временным паролем: сначала смена пароля.
	ErrMustChangePassword = errors.New("нужно сменить временный пароль")
	// ErrWrongPassword — неверный текущий пароль при смене.
	ErrWrongPassword = errors.New("неверный текущий пароль")
	// ErrSetupExpired — ссылка первого входа неверна, уже использована или устарела.
	ErrSetupExpired = errors.New("ссылка устарела")
)

// Account — кто вошёл в кабинет.
type Account struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

func (a Account) IsAdmin() bool { return a.Role == RoleAdmin }

func accountOf(a store.Admin) Account {
	return Account{ID: a.ID, Email: a.Email, Name: a.Name, Role: a.Role, MustChangePassword: a.MustChangePassword}
}

// dummyHash — чтобы время ответа не выдавало, есть ли такая почта.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// checkPassword — правила нового пароля: от 8 символов, не больше 72 байт (предел bcrypt).
func checkPassword(f fields, key, pw string) {
	switch {
	case strings.TrimSpace(pw) == "" || utf8.RuneCountInString(pw) < 8:
		f.add(key, "Пароль — не короче 8 символов")
	case len(pw) > 72:
		f.add(key, "Пароль слишком длинный — до 72 символов")
	}
}

func hashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func (s *Service) newSession(ctx context.Context, adminID int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.q.CreateSession(ctx, store.CreateSessionParams{TokenHash: hashToken(token), AdminID: adminID, ExpiresAt: s.now().Add(SessionTTL)}); err != nil {
		return "", err
	}
	_ = s.q.TouchLogin(ctx, adminID)
	_ = s.q.DeleteExpiredSessions(ctx)
	return token, nil
}

// Login проверяет почту и пароль и возвращает токен сессии для cookie.
// Отключённый сотрудник и ещё не заданный пароль — тот же ответ, что и неверный пароль.
func (s *Service) Login(ctx context.Context, email, password string) (string, Account, error) {
	a, err := s.q.GetAdminByEmail(ctx, normEmail(email))
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", Account{}, ErrBadLogin
	}
	if err != nil {
		return "", Account{}, err
	}
	if a.PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", Account{}, ErrBadLogin
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil || !a.Active {
		return "", Account{}, ErrBadLogin
	}
	token, err := s.newSession(ctx, a.ID)
	if err != nil {
		return "", Account{}, err
	}
	return token, accountOf(a), nil
}

// Session возвращает того, кто вошёл, по токену или ErrUnauthorized (нет сессии, истекла, сотрудник отключён).
func (s *Service) Session(ctx context.Context, token string) (Account, error) {
	if token == "" {
		return Account{}, ErrUnauthorized
	}
	row, err := s.q.GetSession(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrUnauthorized
	}
	if err != nil {
		return Account{}, err
	}
	return Account{ID: row.AdminID, Email: row.Email, Name: row.Name, Role: row.Role, MustChangePassword: row.MustChangePassword}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, hashToken(token))
}

// ChangePassword — смена своего пароля; остальные сессии этого человека закрываются.
// Сразу после входа с временным паролем текущий не спрашиваем (его только что проверили при входе);
// в остальных случаях нужен текущий пароль.
func (s *Service) ChangePassword(ctx context.Context, acc Account, sessionToken, current, next string) error {
	a, err := s.q.GetAdminByID(ctx, acc.ID)
	if err != nil {
		return notFound(err)
	}
	if !a.MustChangePassword && bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(current)) != nil {
		return ErrWrongPassword
	}
	f := fields{}
	checkPassword(f, "new_password", next)
	if f["new_password"] == "" && bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(next)) == nil {
		f.add("new_password", "Новый пароль должен отличаться от прежнего")
	}
	if err := f.err(); err != nil {
		return err
	}
	h, err := hashPassword(next)
	if err != nil {
		return err
	}
	if err := s.inTx(ctx, func(q *store.Queries) error {
		if err := q.SetOwnPassword(ctx, store.SetOwnPasswordParams{ID: a.ID, PasswordHash: h}); err != nil {
			return err
		}
		return q.DeleteOtherSessions(ctx, store.DeleteOtherSessionsParams{AdminID: a.ID, TokenHash: hashToken(sessionToken)})
	}); err != nil {
		return err
	}
	if !a.MustChangePassword {
		s.mailSecurity(ctx, a.Email, "Пароль от кабинета КофеСобакен изменён",
			"Пароль для входа в кабинет только что сменили в разделе «Профиль». На других устройствах вход закрыт.")
	}
	return nil
}

// ChangeEmail — смена своей почты (логина) в «Профиле»: нужен текущий пароль; почта — уникальна среди входов.
// Остальные сессии человека закрываются; на прежнюю почту — письмо-уведомление (если почта подключена).
func (s *Service) ChangeEmail(ctx context.Context, acc Account, sessionToken, current, email string) (Account, error) {
	a, err := s.q.GetAdminByID(ctx, acc.ID)
	if err != nil {
		return Account{}, notFound(err)
	}
	if a.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(current)) != nil {
		return Account{}, ErrWrongPassword
	}
	email = normEmail(email)
	f := fields{}
	checkEmail(f, "email", email)
	if f["email"] == "" && email == a.Email {
		f.add("email", "Это ваша нынешняя почта")
	}
	if err := f.err(); err != nil {
		return Account{}, err
	}
	var out store.Admin
	err = s.inTx(ctx, func(q *store.Queries) error {
		if out, err = q.SetOwnEmail(ctx, store.SetOwnEmailParams{ID: a.ID, Email: email}); err != nil {
			return err
		}
		return q.DeleteOtherSessions(ctx, store.DeleteOtherSessionsParams{AdminID: a.ID, TokenHash: hashToken(sessionToken)})
	})
	if isUnique(err) {
		return Account{}, errEmailTaken
	}
	if err != nil {
		return Account{}, err
	}
	s.mailSecurity(ctx, a.Email, "Почта для входа в кабинет КофеСобакен изменена",
		"Почту для входа в кабинет только что сменили в разделе «Профиль» на "+email+". На других устройствах вход закрыт.")
	return accountOf(out), nil
}

// mailSecurity — письмо о смене пароля или почты. Без почтового ключа — ничего; ошибка — только в лог.
func (s *Service) mailSecurity(ctx context.Context, to, title, body string) {
	ch, ok := s.mailer()
	if !ok {
		return
	}
	body += "\nЕсли это были не вы — сразу напишите в студию или администратору кофейни."
	if err := ch.Send(ctx, to, notify.Message{Kind: "security", Title: title, Body: body}); err != nil {
		logErr("письмо о смене входа", err)
	}
}

// adminByLogin — вход по почте; если почту сменили в «Профиле» — по прежней (ADMIN_EMAIL в окружении остаётся старым).
func (s *Service) adminByLogin(ctx context.Context, email string) (store.Admin, error) {
	a, err := s.q.GetAdminByEmail(ctx, normEmail(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return s.q.GetAdminByFormerEmail(ctx, normEmail(email))
	}
	return a, err
}

// IssueSetupLink создаёт администратору одноразовый код первого входа (старый код перестаёт действовать).
// В базе — только хэш кода; сам код показывается один раз (в логе сервера) ссылкой /admin#setup/<код>.
func (s *Service) IssueSetupLink(ctx context.Context, email string) (string, error) {
	a, err := s.adminByLogin(ctx, email)
	if err != nil {
		return "", notFound(err)
	}
	if a.Role != RoleAdmin {
		return "", ErrForbidden
	}
	code, err := randomToken()
	if err != nil {
		return "", err
	}
	exp := s.now().Add(SetupTTL)
	if err := s.q.SetSetupToken(ctx, store.SetSetupTokenParams{ID: a.ID, SetupTokenHash: pgtype.Text{String: hashToken(code), Valid: true}, SetupExpiresAt: &exp}); err != nil {
		return "", err
	}
	return code, nil
}

// SetupEmail — чья ссылка первого входа (для подсказки на экране); ErrSetupExpired, если ссылка не действует.
func (s *Service) SetupEmail(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrSetupExpired
	}
	a, err := s.q.GetAdminBySetupToken(ctx, pgtype.Text{String: hashToken(code), Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSetupExpired
	}
	return a.Email, err
}

// SetupPassword — первый вход администратора по одноразовой ссылке: свой пароль и сразу вход.
func (s *Service) SetupPassword(ctx context.Context, code, password string) (string, Account, error) {
	if code == "" {
		return "", Account{}, ErrSetupExpired
	}
	a, err := s.q.GetAdminBySetupToken(ctx, pgtype.Text{String: hashToken(code), Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Account{}, ErrSetupExpired
	}
	if err != nil {
		return "", Account{}, err
	}
	if !a.Active {
		return "", Account{}, ErrSetupExpired
	}
	f := fields{}
	checkPassword(f, "password", password)
	if err := f.err(); err != nil {
		return "", Account{}, err
	}
	h, err := hashPassword(password)
	if err != nil {
		return "", Account{}, err
	}
	if err := s.inTx(ctx, func(q *store.Queries) error {
		if err := q.SetOwnPassword(ctx, store.SetOwnPasswordParams{ID: a.ID, PasswordHash: h}); err != nil {
			return err
		}
		return q.DeleteAdminSessions(ctx, a.ID)
	}); err != nil {
		return "", Account{}, err
	}
	token, err := s.newSession(ctx, a.ID)
	if err != nil {
		return "", Account{}, err
	}
	a.MustChangePassword = false
	return token, accountOf(a), nil
}

// mailer — настоящий почтовый канал (Resend); false — ключа нет, письма только в журнал.
func (s *Service) mailer() (notify.Channel, bool) {
	for _, ch := range s.channels {
		if ch.Name() == "email" && !notify.IsLog(ch) {
			return ch, true
		}
	}
	return nil, false
}

// MailConfigured — письма уходят по-настоящему (задан RESEND_API_KEY).
func (s *Service) MailConfigured() bool {
	_, ok := s.mailer()
	return ok
}

// ForgotPassword — письмо со ссылкой на новый пароль (1 час, одноразовая) администратору или сотруднику.
// Неизвестная почта или отключённый сотрудник — молча ничего, чтобы по ответу нельзя было узнать, есть ли такая почта.
// Без почтового ключа — ничего (пароль сотруднику сбрасывает администратор, администратору — admin-link).
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	ch, ok := s.mailer()
	if !ok || s.publicURL == "" {
		return nil
	}
	a, err := s.q.GetAdminByEmail(ctx, normEmail(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !a.Active {
		return nil
	}
	code, err := randomToken()
	if err != nil {
		return err
	}
	exp := s.now().Add(ResetTTL)
	if err := s.q.SetSetupToken(ctx, store.SetSetupTokenParams{ID: a.ID, SetupTokenHash: pgtype.Text{String: hashToken(code), Valid: true}, SetupExpiresAt: &exp}); err != nil {
		return err
	}
	return ch.Send(ctx, a.Email, notify.Message{
		Kind:  "reset",
		Title: "Новый пароль для кабинета КофеСобакен",
		Body:  "Кто-то (наверное, вы) попросил новый пароль для входа в кабинет.\nСсылка действует 1 час и сработает один раз.\nЕсли это были не вы — просто удалите письмо, пароль останется прежним.",
		Link:  s.publicURL + "/admin#setup/" + code,
	})
}

// MailSetupLink — ссылка первого входа администратору на почту (если почта настроена). Ошибка — только в лог:
// ссылка всё равно есть в логе сервера.
func (s *Service) MailSetupLink(ctx context.Context, email, link string) {
	ch, ok := s.mailer()
	if !ok || email == "" {
		return
	}
	if a, err := s.adminByLogin(ctx, email); err == nil {
		email = a.Email // почту сменили в «Профиле» — ссылка уходит на нынешнюю
	}
	err := ch.Send(ctx, normEmail(email), notify.Message{
		Kind:  "setup",
		Title: "Вход в кабинет КофеСобакен",
		Body:  "По ссылке вы зададите пароль администратора.\nСсылка одноразовая и действует 72 часа.",
		Link:  link,
	})
	if err != nil {
		logErr("письмо со ссылкой входа", err)
	}
}
