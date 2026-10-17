package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// YooKassaAPI — адрес API ЮKassa v3. Тестовый магазин работает через тот же адрес, отличаются только ключи.
const YooKassaAPI = "https://api.yookassa.ru/v3"

// YooKassa — адаптер ЮKassa (https://yookassa.ru/developers/api).
// Платёж: POST /payments с подтверждением redirect, capture=true (одностадийная оплата), Idempotence-Key = наш id платежа.
// Уведомления подписи не имеют, поэтому данным из уведомления не доверяем: статус, сумму и наш id берём
// повторным запросом объекта по API (так рекомендует ЮKassa).
type YooKassa struct {
	BaseURL   string
	ShopID    string
	SecretKey string
	// Receipt — отправлять чек 54-ФЗ через ЮKassa. Самозанятый пробивает чек в «Мой налог» сам — false.
	// Для ИП с подключёнными чеками ЮKassa — true.
	Receipt bool
	VATCode int // код ставки НДС для чека (1 — без НДС)
	Client  *http.Client
}

func NewYooKassa(shopID, secretKey string) *YooKassa {
	return &YooKassa{BaseURL: YooKassaAPI, ShopID: shopID, SecretKey: secretKey, VATCode: 1, Client: &http.Client{Timeout: 20 * time.Second}}
}

func (y *YooKassa) Name() string { return "yookassa" }

// TestMode — ключ тестового магазина (начинается с test_).
func (y *YooKassa) TestMode() bool { return strings.HasPrefix(y.SecretKey, "test_") }

type ykAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type ykPayment struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Amount       ykAmount          `json:"amount"`
	Metadata     map[string]string `json:"metadata"`
	Confirmation struct {
		Type            string `json:"type"`
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
}

type ykRefund struct {
	ID        string   `json:"id"`
	PaymentID string   `json:"payment_id"`
	Status    string   `json:"status"`
	Amount    ykAmount `json:"amount"`
}

func rubles(kop int64) ykAmount {
	return ykAmount{Value: fmt.Sprintf("%d.%02d", kop/100, kop%100), Currency: "RUB"}
}

// kopecks разбирает сумму ЮKassa ("500.00", "500.5", "500") в копейки.
func kopecks(a ykAmount) (int64, error) {
	if a.Currency != "RUB" {
		return 0, fmt.Errorf("валюта %q", a.Currency)
	}
	rub, frac, _ := strings.Cut(a.Value, ".")
	if len(frac) > 2 {
		return 0, fmt.Errorf("сумма %q", a.Value)
	}
	frac = (frac + "00")[:2]
	r, err1 := strconv.ParseInt(rub, 10, 64)
	k, err2 := strconv.ParseInt(frac, 10, 64)
	if err1 != nil || err2 != nil || r < 0 {
		return 0, fmt.Errorf("сумма %q", a.Value)
	}
	return r*100 + k, nil
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// APIError — ответ ЮKassa с ошибкой.
type APIError struct {
	Status      int
	Code        string `json:"code"`
	Description string `json:"description"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ЮKassa: ответ %d %s: %s", e.Status, e.Code, e.Description)
}

func (y *YooKassa) call(ctx context.Context, method, path, idemKey string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(y.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(y.ShopID, y.SecretKey)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotence-Key", idemKey)
	}
	resp, err := y.Client.Do(req)
	if err != nil {
		return fmt.Errorf("ЮKassa: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("ЮKassa: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// 202 — ЮKassa ещё обрабатывает запрос, 500 — результат неизвестен: оба случая — ошибка,
		// повтор с тем же Idempotence-Key безопасен.
		e := &APIError{Status: resp.StatusCode}
		_ = json.Unmarshal(raw, e)
		return e
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("ЮKassa: разбор ответа: %w", err)
		}
	}
	return nil
}

func (y *YooKassa) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	in := map[string]any{
		"amount":       rubles(req.AmountKop),
		"capture":      true,
		"confirmation": map[string]string{"type": "redirect", "return_url": req.ReturnURL},
		"description":  cut(req.Description, 128),
		"metadata":     map[string]string{"payment_id": req.PaymentID.String()},
	}
	if y.Receipt {
		if r := y.receipt(req); r != nil {
			in["receipt"] = r
		}
	}
	var p ykPayment
	if err := y.call(ctx, http.MethodPost, "/payments", req.PaymentID.String(), in, &p); err != nil {
		return CreateResult{}, err
	}
	if p.ID == "" || p.Confirmation.ConfirmationURL == "" {
		return CreateResult{}, errors.New("ЮKassa: в ответе нет id или ссылки на оплату")
	}
	return CreateResult{ProviderPaymentID: p.ID, ConfirmationURL: p.Confirmation.ConfirmationURL}, nil
}

// receipt — чек 54-ФЗ одной позицией (для ИП с чеками через ЮKassa). Без телефона гостя чек не собрать — nil.
func (y *YooKassa) receipt(req CreateRequest) map[string]any {
	var phone strings.Builder
	for _, r := range req.Phone {
		if r >= '0' && r <= '9' {
			phone.WriteRune(r)
		}
	}
	if phone.Len() == 0 {
		return nil
	}
	return map[string]any{
		"customer": map[string]string{"phone": phone.String()},
		"items": []map[string]any{{
			"description":     cut(req.Description, 128),
			"quantity":        "1.00",
			"amount":          rubles(req.AmountKop),
			"vat_code":        y.VATCode,
			"payment_mode":    "full_payment",
			"payment_subject": "service",
		}},
	}
}

func (y *YooKassa) payment(ctx context.Context, id string) (ykPayment, error) {
	var p ykPayment
	if id == "" || strings.ContainsAny(id, "/?#") {
		return p, ErrBadSignature
	}
	err := y.call(ctx, http.MethodGet, "/payments/"+id, "", nil, &p)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return p, ErrBadSignature // такого платежа в магазине нет — уведомление поддельное
	}
	return p, err
}

// eventOf — событие по объекту платежа, полученному из API. Незавершённый платёж — ErrIgnored.
func eventOf(p ykPayment) (Event, error) {
	var t string
	switch p.Status {
	case "succeeded":
		t = EventSucceeded
	case "canceled":
		t = EventCanceled
	default:
		return Event{}, ErrIgnored
	}
	id, err := uuid.Parse(p.Metadata["payment_id"])
	if err != nil {
		return Event{}, ErrIgnored // платёж создан не сайтом (например, вручную в кабинете ЮKassa)
	}
	amount, err := kopecks(p.Amount)
	if err != nil {
		return Event{}, fmt.Errorf("ЮKassa: %w", err)
	}
	return Event{Type: t, PaymentID: id, ProviderPaymentID: p.ID, AmountKop: amount}, nil
}

func (y *YooKassa) Check(ctx context.Context, providerPaymentID string) (Event, error) {
	p, err := y.payment(ctx, providerPaymentID)
	if err != nil {
		return Event{}, err
	}
	return eventOf(p)
}

// ParseWebhook: из уведомления берём только тип события и id объекта, остальное — повторным запросом к API.
func (y *YooKassa) ParseWebhook(ctx context.Context, _ http.Header, body []byte) (Event, error) {
	var n struct {
		Type   string `json:"type"`
		Event  string `json:"event"`
		Object struct {
			ID string `json:"id"`
		} `json:"object"`
	}
	if err := json.Unmarshal(body, &n); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if n.Type != "notification" || n.Object.ID == "" {
		return Event{}, fmt.Errorf("%w: не уведомление ЮKassa", ErrMalformed)
	}
	switch n.Event {
	case EventSucceeded, EventCanceled:
		p, err := y.payment(ctx, n.Object.ID)
		if err != nil {
			return Event{}, err
		}
		e, err := eventOf(p)
		if err == nil && e.Type != n.Event {
			return Event{}, ErrBadSignature // статус по API не совпал с уведомлением
		}
		return e, err
	case EventRefunded:
		if strings.ContainsAny(n.Object.ID, "/?#") {
			return Event{}, ErrBadSignature
		}
		var r ykRefund
		err := y.call(ctx, http.MethodGet, "/refunds/"+n.Object.ID, "", nil, &r)
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return Event{}, ErrBadSignature
		}
		if err != nil {
			return Event{}, err
		}
		if r.Status != "succeeded" {
			return Event{}, ErrBadSignature
		}
		p, err := y.payment(ctx, r.PaymentID)
		if err != nil {
			return Event{}, err
		}
		id, err := uuid.Parse(p.Metadata["payment_id"])
		if err != nil {
			return Event{}, ErrIgnored
		}
		amount, err := kopecks(r.Amount)
		if err != nil {
			return Event{}, fmt.Errorf("ЮKassa: %w", err)
		}
		return Event{Type: EventRefunded, PaymentID: id, ProviderPaymentID: p.ID, AmountKop: amount}, nil
	default:
		return Event{}, ErrIgnored // другие события (waiting_for_capture, refund.canceled …) сайту не нужны
	}
}

// Refund возвращает всю сумму платежа. Idempotence-Key привязан к платежу: повтор не создаст второй возврат.
func (y *YooKassa) Refund(ctx context.Context, providerPaymentID string, amountKop int64) error {
	in := map[string]any{"payment_id": providerPaymentID, "amount": rubles(amountKop)}
	var r ykRefund
	if err := y.call(ctx, http.MethodPost, "/refunds", cut("refund-"+providerPaymentID, 64), in, &r); err != nil {
		return err
	}
	if r.Status == "canceled" {
		return errors.New("ЮKassa отклонила возврат")
	}
	return nil
}
