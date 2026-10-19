package service

import (
	"context"
	"fmt"
	"log/slog"
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
	Offset    int64     `json:"offset,omitempty"` // устарело: offset getUpdates теперь в keyTgOffset
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
	s.botName.Store(bot)
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

// TelegramCheck — нажала ли владелица «Старт» по ссылке подключения. Кабинет вызывает раз в несколько секунд, пока ждёт.
// Сообщения боту читает фоновый опрос (RunTelegram); без него (тесты, команда admin-link) — читаем здесь, пока
// ждём «Старт».
func (s *Service) TelegramCheck(ctx context.Context) (Settings, error) {
	if _, ok := s.telegram(); !ok || s.tgPolling.Load() {
		return s.Settings(ctx)
	}
	link, err := getJSON(ctx, s.q, keyTgLink, tgLink{})
	if err != nil {
		return Settings{}, err
	}
	if link.CodeHash == "" || s.now().After(link.ExpiresAt) {
		return s.Settings(ctx)
	}
	if err := s.PollTelegram(ctx, 0); err != nil {
		logErr("Telegram getUpdates", err)
	}
	return s.Settings(ctx)
}

// connectOwner — «Старт» с действующим кодом: этот чат — владелицы, туда идут уведомления и вопросы гостей.
func (s *Service) connectOwner(ctx context.Context, u notify.Update, link tgLink) error {
	ns, err := s.notifySettings(ctx)
	if err != nil {
		return err
	}
	link.CodeHash = ""
	ns.TelegramConnected, ns.TelegramChatID, ns.TelegramChat = true, strconv.FormatInt(u.ChatID, 10), u.Who()
	if err := s.inTx(ctx, func(q *store.Queries) error {
		if err := putJSON(ctx, q, keyTgLink, link); err != nil {
			return err
		}
		return putJSON(ctx, q, keyNotify, ns)
	}); err != nil {
		return err
	}
	if _, err := s.TestNotify(ctx, "telegram"); err != nil {
		logErr("проверочное сообщение Telegram", err)
	}
	return nil
}

const keyTgOffset = "telegram_offset"

// PollTelegram — один разбор входящих боту (getUpdates): подключение владелицы, вопросы гостей, её реплаи.
// wait — сколько секунд Telegram ждёт новых сообщений. Offset — в базе: после перезапуска сервер продолжает
// с того же места, а прочитанное Telegram больше не отдаёт.
func (s *Service) PollTelegram(ctx context.Context, wait int) error {
	tg, ok := s.telegram()
	if !ok {
		return nil
	}
	s.tgMu.Lock()
	defer s.tgMu.Unlock()
	offset, err := getJSON[int64](ctx, s.q, keyTgOffset, 0)
	if err != nil {
		return err
	}
	if offset == 0 { // до задачи «вопросы» offset лежал в ссылке подключения
		link, err := getJSON(ctx, s.q, keyTgLink, tgLink{})
		if err != nil {
			return err
		}
		offset = link.Offset
	}
	ups, next, err := tg.Updates(ctx, offset, wait)
	if err != nil {
		return err
	}
	var herr error
	for _, u := range ups {
		if herr = s.handleUpdate(ctx, tg, u); herr != nil {
			next = u.UpdateID // разберём это сообщение ещё раз
			break
		}
	}
	if next != offset {
		if err := putJSON(ctx, s.q, keyTgOffset, next); err != nil {
			return err
		}
	}
	return herr
}

// RunTelegram — фоновый опрос бота (длинный: Telegram держит запрос до 25 секунд и отвечает сразу, как придёт
// сообщение). Без вебхука: работает за любым адресом и не теряет сообщения, пока сервер спал.
func (s *Service) RunTelegram(ctx context.Context) {
	tg, ok := s.telegram()
	if !ok {
		return
	}
	s.tgPolling.Store(true)
	defer s.tgPolling.Store(false)
	pause := 5 * time.Second
	for ctx.Err() == nil {
		if s.BotName() == "" {
			if name, err := tg.BotUsername(ctx); err == nil {
				s.botName.Store(name)
			}
		}
		if err := s.PollTelegram(ctx, 25); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("Telegram: входящие не прочитаны", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(pause):
			}
			pause = min(pause*2, 2*time.Minute)
			continue
		}
		pause = 5 * time.Second
	}
}

// BotName — имя бота Telegram (для ссылки «Написать в Telegram» на сайте); пусто — бот не настроен.
func (s *Service) BotName() string {
	v, _ := s.botName.Load().(string)
	return v
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
