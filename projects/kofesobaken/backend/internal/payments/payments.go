// Package payments — оплата на сайте за общим интерфейсом.
// Сейчас подключена только тестовая заглушка; настоящая платёжная система добавляется
// новым адаптером, остальной код не меняется.
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
)

var ErrBadSignature = errors.New("неверная подпись уведомления")

type CreateRequest struct {
	PaymentID   uuid.UUID
	AmountKop   int64
	Description string
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
	// ParseWebhook проверяет подпись и разбирает уведомление.
	ParseWebhook(h http.Header, body []byte) (Event, error)
	Refund(ctx context.Context, providerPaymentID string, amountKop int64) error
}
