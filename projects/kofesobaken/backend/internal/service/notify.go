package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/store"
)

// dispatch отправляет сообщение во все включённые каналы и пишет результат в журнал.
// Ошибка отправки не ломает оформление: гость уже оплатил, сообщение остаётся в журнале.
func (s *Service) dispatch(ctx context.Context, m notify.Message, force bool) []store.Notification {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	ns, err := s.notifySettings(ctx)
	if err != nil {
		logErr("настройки уведомлений", err)
		return nil
	}
	var out []store.Notification
	for _, ch := range s.channels {
		var to string
		switch ch.Name() {
		case "email":
			if !ns.EmailEnabled && !force || ns.Email == "" {
				continue
			}
			to = ns.Email
		case "telegram":
			if !ns.TelegramConnected {
				continue
			}
			to = ns.tgRecipient()
		}
		status := "sent"
		if err := ch.Send(ctx, to, m); err != nil {
			logErr("уведомление не отправлено", err, "channel", ch.Name(), "kind", m.Kind)
			status = "failed"
		}
		n, err := s.q.AddNotification(ctx, store.AddNotificationParams{Channel: ch.Name(), Kind: m.Kind, Recipient: to, Title: m.Title, Body: m.Body, Status: status})
		if err != nil {
			logErr("журнал уведомлений", err)
			continue
		}
		out = append(out, n)
	}
	return out
}

func (s *Service) enabled(ctx context.Context, pick func(NotifySettings) bool) bool {
	ns, err := s.notifySettings(ctx)
	if err != nil {
		logErr("настройки уведомлений", err)
		return false
	}
	return pick(ns)
}

func guests(n int) string {
	return fmt.Sprintf("%d %s", n, Plural(n, "гость", "гостя", "гостей"))
}

func (s *Service) notifyBooking(ctx context.Context, b store.Booking) {
	if !s.enabled(ctx, func(n NotifySettings) bool { return n.OnBooking }) {
		return
	}
	parts := []string{dayTime(b.Day, Clock(b.At)), guests(int(b.Guests))}
	if b.WithDog {
		parts = append(parts, "с собакой")
	}
	s.dispatch(ctx, notify.Message{
		Kind:    "booking",
		Title:   "Новая бронь столика",
		Body:    fmt.Sprintf("%s · %s\n%s\nПредоплата %s оплачена", b.Name, b.Phone, strings.Join(parts, " · "), Rub(b.PrepayKop)),
		Buttons: []string{"Подтвердить", "Позвонить"},
		Phone:   b.Phone,
		Link:    s.cabinet("bookings"),
	}, false)
}

func (s *Service) notifyOrder(ctx context.Context, o store.Order, items []store.OrderItem) {
	if !s.enabled(ctx, func(n NotifySettings) bool { return n.OnOrder }) {
		return
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, fmt.Sprintf("%s × %d", it.Name, it.Qty))
	}
	how := "Заберёт сам"
	if o.Mode == "delivery" {
		how = "Доставка: " + o.Address
	}
	s.dispatch(ctx, notify.Message{
		Kind:    "order",
		Title:   fmt.Sprintf("Новый заказ №%d", o.ID),
		Body:    fmt.Sprintf("%s\n%s · %s\nОплачено %s", strings.Join(lines, ", "), how, o.Phone, Rub(o.TotalKop)),
		Buttons: []string{"Позвонить"},
		Phone:   o.Phone,
		Link:    s.cabinet("orders"),
	}, false)
}

func (s *Service) notifySignup(ctx context.Context, ev store.Event, su store.EventSignup) {
	if s.enabled(ctx, func(n NotifySettings) bool { return n.OnSignup }) {
		body := fmt.Sprintf("%s · %s\n%s — %s\n%d чел.", su.Name, su.Contact, ev.Title, dayTime(ev.Day, Clock(ev.At)), su.People)
		if su.WithDog {
			body += " · с собакой"
		}
		if su.AmountKop > 0 {
			body += "\nОплачено " + Rub(su.AmountKop)
		}
		m := notify.Message{Kind: "signup", Title: "Запись на мероприятие", Body: body, Link: s.cabinet("events")}
		if su.Via != "tg" {
			m.Phone = su.Contact
		}
		s.dispatch(ctx, m, false)
	}
	taken, err := s.q.EventTaken(ctx, ev.ID)
	if err != nil {
		logErr("места мероприятия", err)
		return
	}
	if taken >= ev.Seats && s.enabled(ctx, func(n NotifySettings) bool { return n.OnFull }) {
		s.dispatch(ctx, notify.Message{Kind: "full", Title: "Мест больше нет", Body: fmt.Sprintf("%s — %s: все %d мест заняты", ev.Title, dayTime(ev.Day, Clock(ev.At)), ev.Seats), Link: s.cabinet("events")}, false)
	}
}

// TestNotify — проверочное сообщение в выбранный канал.
func (s *Service) TestNotify(ctx context.Context, channel string) (store.Notification, error) {
	ns, err := s.notifySettings(ctx)
	if err != nil {
		return store.Notification{}, err
	}
	switch channel {
	case "email":
		if ns.Email == "" {
			return store.Notification{}, &ValidationError{Fields: map[string]string{"email": "Укажите почту"}}
		}
	case "telegram":
		if !ns.TelegramConnected {
			return store.Notification{}, &ConflictError{Msg: "Сначала подключите Telegram"}
		}
	default:
		return store.Notification{}, &ValidationError{Fields: map[string]string{"channel": "Неизвестный канал"}}
	}
	m := notify.Message{Kind: "test", Title: "Проверка уведомлений", Body: "Это проверочное сообщение с сайта КофеСобакен. Всё работает.", Link: s.cabinet("notify")}
	for _, ch := range s.channels {
		if ch.Name() != channel {
			continue
		}
		to := ns.Email
		if channel == "telegram" {
			to = ns.tgRecipient()
		}
		status := "sent"
		if err := ch.Send(ctx, to, m); err != nil {
			status = "failed"
		}
		return s.q.AddNotification(ctx, store.AddNotificationParams{Channel: channel, Kind: m.Kind, Recipient: to, Title: m.Title, Body: m.Body, Status: status})
	}
	return store.Notification{}, &ConflictError{Msg: "Канал не настроен"}
}

// Подключение Telegram. С ботом (TELEGRAM_BOT_TOKEN): «Подключить» выдаёт одноразовую ссылку
// t.me/<бот>?start=<код> (30 минут; в базе — только хэш кода), владелица жмёт «Старт», кабинет опрашивает
// TelegramCheck — сервер читает getUpdates, находит «/start <код>» и запоминает chat_id.
// Без бота — заглушка, как раньше: считаем подключённым и пишем сообщения в журнал.

const (
	keyTgLink       = "telegram_link"
	TelegramLinkTTL = 30 * time.Minute
)

type tgLink struct {
	CodeHash  string    `json:"code_hash,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Offset    int64     `json:"offset"` // getUpdates: всё до него уже прочитано
}

// TelegramConnectResult — настройки и ссылка на бота (пусто — бот не настроен, подключено заглушкой).
type TelegramConnectResult struct {
	Settings
	Link string `json:"telegram_link,omitempty"`
}

func (s *Service) telegram() (notify.Telegram, bool) {
	for _, ch := range s.channels {
		if t, ok := ch.(notify.Telegram); ok {
			return t, true
		}
	}
	return notify.Telegram{}, false
}

func (s *Service) TelegramConnect(ctx context.Context) (TelegramConnectResult, error) {
	tg, ok := s.telegram()
	if !ok {
		st, err := s.telegramConnectStub(ctx)
		return TelegramConnectResult{Settings: st}, err
	}
	bot, err := tg.BotUsername(ctx)
	if err != nil {
		logErr("бот Telegram", err)
		return TelegramConnectResult{}, &ConflictError{Msg: "Бот Telegram не отвечает — проверьте TELEGRAM_BOT_TOKEN"}
	}
	code, err := randomToken()
	if err != nil {
		return TelegramConnectResult{}, err
	}
	link, err := getJSON(ctx, s.q, keyTgLink, tgLink{})
	if err != nil {
		return TelegramConnectResult{}, err
	}
	link.CodeHash, link.ExpiresAt = hashToken(code), s.now().Add(TelegramLinkTTL)
	if err := putJSON(ctx, s.q, keyTgLink, link); err != nil {
		return TelegramConnectResult{}, err
	}
	st, err := s.Settings(ctx)
	return TelegramConnectResult{Settings: st, Link: "https://t.me/" + bot + "?start=" + code}, err
}

// TelegramCheck — нажала ли владелица «Старт» по ссылке подключения; если да — чат подключён,
// уходит проверочное сообщение. Кабинет вызывает раз в несколько секунд, пока ждёт.
func (s *Service) TelegramCheck(ctx context.Context) (Settings, error) {
	tg, ok := s.telegram()
	if !ok {
		return s.Settings(ctx)
	}
	link, err := getJSON(ctx, s.q, keyTgLink, tgLink{})
	if err != nil {
		return Settings{}, err
	}
	if link.CodeHash == "" || s.now().After(link.ExpiresAt) {
		return s.Settings(ctx)
	}
	starts, next, err := tg.Starts(ctx, link.Offset)
	if err != nil {
		logErr("Telegram getUpdates", err)
		return s.Settings(ctx)
	}
	link.Offset = next
	var found *notify.Start
	for i := range starts {
		if hashToken(starts[i].Code) == link.CodeHash {
			found = &starts[i]
		}
	}
	if found == nil {
		if err := putJSON(ctx, s.q, keyTgLink, link); err != nil {
			return Settings{}, err
		}
		return s.Settings(ctx)
	}
	ns, err := s.notifySettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	link.CodeHash = ""
	ns.TelegramConnected, ns.TelegramChatID, ns.TelegramChat = true, strconv.FormatInt(found.ChatID, 10), found.Name
	if err := s.inTx(ctx, func(q *store.Queries) error {
		if err := putJSON(ctx, q, keyTgLink, link); err != nil {
			return err
		}
		return putJSON(ctx, q, keyNotify, ns)
	}); err != nil {
		return Settings{}, err
	}
	if _, err := s.TestNotify(ctx, "telegram"); err != nil {
		logErr("проверочное сообщение Telegram", err)
	}
	return s.Settings(ctx)
}

// telegramConnectStub — без бота: считаем подключённым чат из настроек и шлём проверку в журнал.
func (s *Service) telegramConnectStub(ctx context.Context) (Settings, error) {
	ns, err := s.notifySettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	ns.TelegramConnected = true
	if ns.TelegramChat == "" {
		ns.TelegramChat = DefaultNotify().TelegramChat
	}
	if err := putJSON(ctx, s.q, keyNotify, ns); err != nil {
		return Settings{}, err
	}
	if _, err := s.TestNotify(ctx, "telegram"); err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

func (s *Service) TelegramDisconnect(ctx context.Context) (Settings, error) {
	ns, err := s.notifySettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	ns.TelegramConnected, ns.TelegramChatID = false, ""
	if err := putJSON(ctx, s.q, keyNotify, ns); err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

func (s *Service) ListNotifications(ctx context.Context) ([]store.Notification, error) {
	return s.q.ListNotifications(ctx, 50)
}

// cabinet — ссылка на раздел кабинета ("" — адрес сайта не задан).
func (s *Service) cabinet(section string) string {
	if s.publicURL == "" {
		return ""
	}
	return s.publicURL + "/admin#" + section
}

// tgRecipient — chat_id из подключения бота; без бота (заглушка) — имя для журнала.
func (n NotifySettings) tgRecipient() string {
	if n.TelegramChatID != "" {
		return n.TelegramChatID
	}
	return n.TelegramChat
}
