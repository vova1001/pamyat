// Package notify — уведомления владельцу (почта, Telegram) за общим интерфейсом.
// Сейчас адаптеры — заглушки: пишут сообщение в лог. Настоящая отправка подключается
// новым адаптером без изменения остального кода.
package notify

import (
	"context"
	"log/slog"
)

type Message struct {
	Kind    string // booking, order, signup, full, test
	Title   string
	Body    string
	Buttons []string
}

type Channel interface {
	Name() string // email | telegram
	Send(ctx context.Context, recipient string, m Message) error
}

// LogChannel — заглушка канала: пишет сообщение в лог.
type LogChannel struct{ Channel_ string }

func (l LogChannel) Name() string { return l.Channel_ }

func (l LogChannel) Send(_ context.Context, recipient string, m Message) error {
	slog.Info("уведомление (заглушка)", "channel", l.Channel_, "to", recipient, "kind", m.Kind, "title", m.Title, "body", m.Body, "buttons", m.Buttons)
	return nil
}
