package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// TestProvider — тестовая заглушка: денег не списывает. Страница «оплаты» — /pay/<id> на сайте;
// по кнопке заглушка присылает подписанное уведомление на webhook, как настоящая система.
type TestProvider struct {
	PublicURL  string
	WebhookURL string
	Secret     []byte
	Client     *http.Client
}

func NewTestProvider(publicURL, webhookURL, secret string) *TestProvider {
	return &TestProvider{PublicURL: publicURL, WebhookURL: webhookURL, Secret: []byte(secret), Client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *TestProvider) Name() string { return "test" }

func (p *TestProvider) Create(_ context.Context, req CreateRequest) (CreateResult, error) {
	return CreateResult{
		ProviderPaymentID: "test_" + req.PaymentID.String(),
		ConfirmationURL:   p.PublicURL + "/pay/" + req.PaymentID.String(),
	}, nil
}

// Sign — подпись тела уведомления (HMAC-SHA256, hex).
func Sign(secret, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func (p *TestProvider) ParseWebhook(h http.Header, body []byte) (Event, error) {
	want := Sign(p.Secret, body)
	if !hmac.Equal([]byte(want), []byte(h.Get("X-Signature"))) {
		return Event{}, ErrBadSignature
	}
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		return Event{}, fmt.Errorf("разбор уведомления: %w", err)
	}
	if e.Type != EventSucceeded && e.Type != EventCanceled {
		return Event{}, fmt.Errorf("неизвестное событие %q", e.Type)
	}
	return e, nil
}

func (p *TestProvider) Refund(_ context.Context, providerPaymentID string, amountKop int64) error {
	slog.Info("оплата (заглушка): возврат", "provider_payment_id", providerPaymentID, "amount_kop", amountKop)
	return nil
}

// Simulate отправляет на webhook подписанное уведомление — как будто гость оплатил или отказался.
func (p *TestProvider) Simulate(ctx context.Context, eventType string, id uuid.UUID, providerID string, amountKop int64) error {
	body, _ := json.Marshal(Event{Type: eventType, PaymentID: id, ProviderPaymentID: providerID, AmountKop: amountKop})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", Sign(p.Secret, body))
	resp, err := p.Client.Do(req)
	if err != nil {
		return fmt.Errorf("уведомление заглушки: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("уведомление заглушки: ответ %d", resp.StatusCode)
	}
	return nil
}
