// Package notify — уведомления владельцу (почта, Telegram) за общим интерфейсом.
// Адаптеры: Resend (почта), Telegram Bot API; без ключей — LogChannel (сообщение в лог).
package notify

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type Message struct {
	Kind    string // booking, order, signup, full, test, reset, setup
	Title   string
	Body    string
	Buttons []string
	Phone   string // телефон гостя — кнопка «Позвонить»
	Link    string // ссылка в кабинет (или ссылка входа — для писем reset/setup)
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

// IsLog — канал-заглушка (ключ сервиса не задан).
func IsLog(c Channel) bool {
	_, ok := c.(LogChannel)
	return ok
}

var defaultClient = &http.Client{Timeout: 15 * time.Second}

func client(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return defaultClient
}
