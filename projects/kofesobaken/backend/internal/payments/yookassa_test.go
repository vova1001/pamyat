package payments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// ykStub — заглушка API ЮKassa на httptest: платежи и возвраты в памяти.
type ykStub struct {
	mu       sync.Mutex
	payments map[string]map[string]any
	refunds  map[string]map[string]any
	idem     map[string]string // Idempotence-Key → id созданного объекта
	last     map[string]any    // тело последнего POST /payments
	fail     int               // ответить этим кодом
}

func newYKStub(t *testing.T) (*ykStub, *YooKassa) {
	st := &ykStub{payments: map[string]map[string]any{}, refunds: map[string]map[string]any{}, idem: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(st.serve))
	t.Cleanup(srv.Close)
	y := NewYooKassa("123456", "test_secret")
	y.BaseURL = srv.URL + "/v3"
	return st, y
}

func (st *ykStub) serve(w http.ResponseWriter, r *http.Request) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != "123456" || p != "test_secret" {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"type":"error","code":"invalid_credentials","description":"bad"}`))
		return
	}
	if st.fail != 0 {
		w.WriteHeader(st.fail)
		return
	}
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.Method == "POST" && r.URL.Path == "/v3/payments":
		key := r.Header.Get("Idempotence-Key")
		if key == "" || len(key) > 64 {
			w.WriteHeader(400)
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		st.last = in
		if id, ok := st.idem[key]; ok {
			reply(st.payments[id])
			return
		}
		id := "pay-" + uuid.NewString()[:8]
		p := map[string]any{"id": id, "status": "pending", "amount": in["amount"], "metadata": in["metadata"],
			"confirmation": map[string]any{"type": "redirect", "confirmation_url": "https://yoomoney.ru/checkout/payments/v2/contract?orderId=" + id}}
		st.payments[id], st.idem[key] = p, id
		reply(p)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v3/payments/"):
		p, ok := st.payments[strings.TrimPrefix(r.URL.Path, "/v3/payments/")]
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"type":"error","code":"not_found"}`))
			return
		}
		reply(p)
	case r.Method == "POST" && r.URL.Path == "/v3/refunds":
		key := r.Header.Get("Idempotence-Key")
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if id, ok := st.idem[key]; ok {
			reply(st.refunds[id])
			return
		}
		id := "ref-" + uuid.NewString()[:8]
		rf := map[string]any{"id": id, "payment_id": in["payment_id"], "status": "succeeded", "amount": in["amount"]}
		st.refunds[id], st.idem[key] = rf, id
		reply(rf)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v3/refunds/"):
		rf, ok := st.refunds[strings.TrimPrefix(r.URL.Path, "/v3/refunds/")]
		if !ok {
			w.WriteHeader(404)
			return
		}
		reply(rf)
	default:
		w.WriteHeader(404)
	}
}

func (st *ykStub) set(id, status string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.payments[id]["status"] = status
}

func notification(event, id string) []byte {
	return []byte(`{"type":"notification","event":"` + event + `","object":{"id":"` + id + `","status":"succeeded"}}`)
}

func TestYooKassaFlow(t *testing.T) {
	st, y := newYKStub(t)
	ctx := context.Background()
	id := uuid.New()
	req := CreateRequest{PaymentID: id, AmountKop: 50050, Description: strings.Repeat("я", 200), ReturnURL: "https://site.ru/?payment=" + id.String(), Phone: "+7 (900) 123-45-67"}
	r, err := y.Create(ctx, req)
	if err != nil || !strings.HasPrefix(r.ConfirmationURL, "https://yoomoney.ru/") || r.ProviderPaymentID == "" {
		t.Fatalf("Create: %v %+v", err, r)
	}
	amount := st.last["amount"].(map[string]any)
	conf := st.last["confirmation"].(map[string]any)
	if amount["value"] != "500.50" || amount["currency"] != "RUB" || st.last["capture"] != true ||
		conf["type"] != "redirect" || conf["return_url"] != req.ReturnURL ||
		len([]rune(st.last["description"].(string))) != 128 || st.last["metadata"].(map[string]any)["payment_id"] != id.String() {
		t.Fatalf("тело запроса: %v", st.last)
	}
	if _, ok := st.last["receipt"]; ok {
		t.Fatal("самозанятый: receipt не отправляется")
	}
	// повтор с тем же ключом — тот же платёж
	if r2, _ := y.Create(ctx, req); r2.ProviderPaymentID != r.ProviderPaymentID {
		t.Fatal("Idempotence-Key: повтор создал второй платёж")
	}

	// пока не оплачен — уведомление и проверка ничего не дают
	if _, err := y.Check(ctx, r.ProviderPaymentID); !errors.Is(err, ErrIgnored) {
		t.Fatalf("pending: %v", err)
	}
	// поддельное «оплачено» при pending по API — отклоняется
	if _, err := y.ParseWebhook(ctx, nil, notification(EventSucceeded, r.ProviderPaymentID)); !errors.Is(err, ErrIgnored) {
		t.Fatalf("pending по API: %v", err)
	}
	// неизвестный платёж — подделка
	if _, err := y.ParseWebhook(ctx, nil, notification(EventSucceeded, "nope")); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("чужой id: %v", err)
	}
	if _, err := y.ParseWebhook(ctx, nil, []byte(`{`)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("мусор: %v", err)
	}
	if _, err := y.ParseWebhook(ctx, nil, notification("payment.waiting_for_capture", r.ProviderPaymentID)); !errors.Is(err, ErrIgnored) {
		t.Fatalf("лишнее событие: %v", err)
	}

	st.set(r.ProviderPaymentID, "succeeded")
	e, err := y.ParseWebhook(ctx, nil, notification(EventSucceeded, r.ProviderPaymentID))
	if err != nil || e.Type != EventSucceeded || e.PaymentID != id || e.AmountKop != 50050 || e.ProviderPaymentID != r.ProviderPaymentID {
		t.Fatalf("succeeded: %v %+v", err, e)
	}
	if _, err := y.ParseWebhook(ctx, nil, notification(EventCanceled, r.ProviderPaymentID)); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("canceled при succeeded по API: %v", err)
	}
	if e, err := y.Check(ctx, r.ProviderPaymentID); err != nil || e.Type != EventSucceeded {
		t.Fatalf("Check: %v %+v", err, e)
	}

	if err := y.Refund(ctx, r.ProviderPaymentID, 50050); err != nil {
		t.Fatal(err)
	}
	if err := y.Refund(ctx, r.ProviderPaymentID, 50050); err != nil || len(st.refunds) != 1 {
		t.Fatalf("повторный возврат: %v, возвратов %d", err, len(st.refunds))
	}
	var refID string
	for k := range st.refunds {
		refID = k
	}
	e, err = y.ParseWebhook(ctx, nil, notification(EventRefunded, refID))
	if err != nil || e.Type != EventRefunded || e.PaymentID != id || e.AmountKop != 50050 {
		t.Fatalf("refund.succeeded: %v %+v", err, e)
	}

	st.fail = 500
	if _, err := y.ParseWebhook(ctx, nil, notification(EventSucceeded, r.ProviderPaymentID)); err == nil || errors.Is(err, ErrMalformed) {
		t.Fatalf("ЮKassa недоступна — ошибка для повтора: %v", err)
	}
	var ae *APIError
	if _, err := y.Create(ctx, CreateRequest{PaymentID: uuid.New(), AmountKop: 100}); !errors.As(err, &ae) || ae.Status != 500 {
		t.Fatalf("500: %v", err)
	}
	st.fail = 0
	y.SecretKey = "wrong"
	if _, err := y.Create(ctx, CreateRequest{PaymentID: uuid.New(), AmountKop: 100}); !errors.As(err, &ae) || ae.Status != 401 || ae.Code != "invalid_credentials" {
		t.Fatalf("401: %v", err)
	}
	if y.Name() != "yookassa" || NewYooKassa("1", "test_x").TestMode() != true || NewYooKassa("1", "live_x").TestMode() {
		t.Fatal("имя / режим")
	}
}

func TestYooKassaReceiptSwitch(t *testing.T) {
	st, y := newYKStub(t)
	y.Receipt = true
	if _, err := y.Create(context.Background(), CreateRequest{PaymentID: uuid.New(), AmountKop: 30000, Description: "Бронь", Phone: "+7 (900) 123-45-67"}); err != nil {
		t.Fatal(err)
	}
	rc, ok := st.last["receipt"].(map[string]any)
	if !ok || rc["customer"].(map[string]any)["phone"] != "79001234567" {
		t.Fatalf("чек: %v", st.last["receipt"])
	}
	item := rc["items"].([]any)[0].(map[string]any)
	if item["vat_code"] != float64(1) || item["quantity"] != "1.00" || item["amount"].(map[string]any)["value"] != "300.00" {
		t.Fatalf("позиция чека: %v", item)
	}
	if _, err := y.Create(context.Background(), CreateRequest{PaymentID: uuid.New(), AmountKop: 30000}); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.last["receipt"]; ok {
		t.Fatal("без телефона чек не собрать — не отправляем")
	}
}

func TestKopecks(t *testing.T) {
	for in, want := range map[string]int64{"500.00": 50000, "500.5": 50050, "500": 50000, "0.01": 1} {
		if got, err := kopecks(ykAmount{Value: in, Currency: "RUB"}); err != nil || got != want {
			t.Fatalf("%s: %d %v", in, got, err)
		}
	}
	for _, in := range []string{"1.001", "-1.00", "abc"} {
		if _, err := kopecks(ykAmount{Value: in, Currency: "RUB"}); err == nil {
			t.Fatalf("%s: должна быть ошибка", in)
		}
	}
	if _, err := kopecks(ykAmount{Value: "1.00", Currency: "USD"}); err == nil {
		t.Fatal("валюта")
	}
}
