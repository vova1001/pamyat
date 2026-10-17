package service

import (
	"context"
	"fmt"
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
			to = ns.TelegramChat
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
		s.dispatch(ctx, notify.Message{Kind: "signup", Title: "Запись на мероприятие", Body: body}, false)
	}
	taken, err := s.q.EventTaken(ctx, ev.ID)
	if err != nil {
		logErr("места мероприятия", err)
		return
	}
	if taken >= ev.Seats && s.enabled(ctx, func(n NotifySettings) bool { return n.OnFull }) {
		s.dispatch(ctx, notify.Message{Kind: "full", Title: "Мест больше нет", Body: fmt.Sprintf("%s — %s: все %d мест заняты", ev.Title, dayTime(ev.Day, Clock(ev.At)), ev.Seats)}, false)
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
	m := notify.Message{Kind: "test", Title: "Проверка уведомлений", Body: "Это проверочное сообщение с сайта КофеСобакен. Всё работает."}
	for _, ch := range s.channels {
		if ch.Name() != channel {
			continue
		}
		to := ns.Email
		if channel == "telegram" {
			to = ns.TelegramChat
		}
		status := "sent"
		if err := ch.Send(ctx, to, m); err != nil {
			status = "failed"
		}
		return s.q.AddNotification(ctx, store.AddNotificationParams{Channel: channel, Kind: m.Kind, Recipient: to, Title: m.Title, Body: m.Body, Status: status})
	}
	return store.Notification{}, &ConflictError{Msg: "Канал не настроен"}
}

// TelegramConnect — заглушка подключения: считаем бот подключённым к чату владельца и шлём проверку.
// Настоящее подключение (бот, «Старт», chat_id) — на шаге регистраций.
func (s *Service) TelegramConnect(ctx context.Context) (Settings, error) {
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
	ns.TelegramConnected = false
	if err := putJSON(ctx, s.q, keyNotify, ns); err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

func (s *Service) ListNotifications(ctx context.Context) ([]store.Notification, error) {
	return s.q.ListNotifications(ctx, 50)
}
