// Package payments — оплата на сайте за общим интерфейсом.
// Адаптеры: тестовая заглушка (TestProvider) и ЮKassa (YooKassa). Новая платёжная система —
// новый адаптер, остальной код не меняется.
package payments

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

// События уведомления (webhook) платёжной системы.
const (
	EventSucceeded = "payment.succeeded"
	EventCanceled  = "payment.canceled"
	EventRefunded  = "refund.succeeded"
)

var (
	// ErrBadSignature — уведомление не подтвердилось (подпись или повторный запрос к платёжной системе).
	ErrBadSignature = errors.New("неверная подпись уведомления")
	// ErrIgnored — уведомление настоящее, но для сайта ничего не меняет (промежуточный статус и т. п.).
	ErrIgnored = errors.New("уведомление не требует действий")
	// ErrMalformed — уведомление не разобрать.
	ErrMalformed = errors.New("неверное уведомление")
)

type CreateRequest struct {
	PaymentID   uuid.UUID
	AmountKop   int64
	Description string
	ReturnURL   string // куда платёжная система вернёт гостя после оплаты
	Phone       string // телефон гостя, если есть (нужен для чека, когда чеки шлёт платёжная система)
}

type CreateResult struct {
	ProviderPaymentID string
	ConfirmationURL   string // страница оплаты для гостя
}

// Event — проверенное уведомление о платеже.
type Event struct {
	Type              string    `json:"event"`
	PaymentID         uuid.UUID `json:"payment_id"`
	ProviderPaymentID string    `json:"provider_payment_id"`
	AmountKop         int64     `json:"amount_kop"`
}

// Provider — общий интерфейс платёжной системы.
type Provider interface {
	Name() string
	Create(ctx context.Context, req CreateRequest) (CreateResult, error)
	// ParseWebhook проверяет подлинность и разбирает уведомление.
	ParseWebhook(ctx context.Context, h http.Header, body []byte) (Event, error)
	Refund(ctx context.Context, providerPaymentID string, amountKop int64) error
}

// Checker — платёжная система умеет отдать текущий статус платежа по запросу
// (проверка при возврате гостя на сайт, если уведомление ещё не дошло).
// Платёж ещё не завершён — ErrIgnored.
type Checker interface {
	Check(ctx context.Context, providerPaymentID string) (Event, error)
}
