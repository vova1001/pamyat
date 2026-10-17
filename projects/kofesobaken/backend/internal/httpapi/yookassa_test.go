package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"kofesobaken/backend/internal/httpapi"
	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
	"kofesobaken/backend/migrations"
)

// yk — заглушка API ЮKassa на httptest: создаёт платежи, «оплачивает» их и шлёт уведомления, как настоящая.
type yk struct {
	mu       sync.Mutex
	payments map[string]map[string]any
	refunds  map[string]map[string]any
	idem     map[string]string
	hook     string
	t        *testing.T
}

func (s *yk) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, p, _ := r.BasicAuth(); u != "shop" || p != "test_key" {
		w.WriteHeader(401)
		return
	}
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	key := r.Header.Get("Idempotence-Key")
	switch {
	case r.Method == "POST" && r.URL.Path == "/v3/payments":
		if key == "" {
			w.WriteHeader(400)
			return
		}
		if _, ok := in["receipt"]; ok {
			s.t.Error("самозанятый: receipt не отправляется")
		}
		if id, ok := s.idem[key]; ok {
			reply(s.payments[id])
			return
		}
		id := "yk-" + uuid.NewString()
		p := map[string]any{"id": id, "status": "pending", "amount": in["amount"], "metadata": in["metadata"],
			"return_url":   in["confirmation"].(map[string]any)["return_url"],
			"confirmation": map[string]any{"type": "redirect", "confirmation_url": "https://yoomoney.test/checkout/" + id}}
		s.payments[id], s.idem[key] = p, id
		reply(p)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v3/payments/"):
		p, ok := s.payments[strings.TrimPrefix(r.URL.Path, "/v3/payments/")]
		if !ok {
			w.WriteHeader(404)
			return
		}
		reply(p)
	case r.Method == "POST" && r.URL.Path == "/v3/refunds":
		if id, ok := s.idem[key]; ok {
			reply(s.refunds[id])
			return
		}
		id := "rf-" + uuid.NewString()
		rf := map[string]any{"id": id, "payment_id": in["payment_id"], "status": "succeeded", "amount": in["amount"]}
		s.refunds[id], s.idem[key] = rf, id
		reply(rf)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v3/refunds/"):
		rf, ok := s.refunds[strings.TrimPrefix(r.URL.Path, "/v3/refunds/")]
		if !ok {
			w.WriteHeader(404)
			return
		}
		reply(rf)
	default:
		w.WriteHeader(404)
	}
}

// byURL — платёж ЮKassa по ссылке оплаты.
func (s *yk) byURL(confirmation string) string {
	return strings.TrimPrefix(confirmation, "https://yoomoney.test/checkout/")
}

func (s *yk) setStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payments[id]["status"] = status
}

// notify шлёт уведомление ЮKassa на webhook сайта и возвращает код ответа.
func (s *yk) notify(event, objectID string) int {
	body := `{"type":"notification","event":"` + event + `","object":{"id":"` + objectID + `"}}`
	res, err := http.Post(s.hook, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		s.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func setupYooKassa(t *testing.T) (*client, *yk) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("нужна TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(pool); err != nil {
		t.Fatal(err)
	}
	stub := &yk{payments: map[string]map[string]any{}, refunds: map[string]map[string]any{}, idem: map[string]string{}, t: t}
	api := httptest.NewServer(stub)
	t.Cleanup(api.Close)
	pay := payments.NewYooKassa("shop", "test_key")
	pay.BaseURL = api.URL + "/v3"
	loc, _ := time.LoadLocation("Europe/Moscow")
	svc := service.New(pool, pay, []notify.Channel{notify.LogChannel{Channel_: "email"}, notify.LogChannel{Channel_: "telegram"}}, loc)
	svc.SetPublicURL("https://kofesobaken.test")
	if err := svc.Bootstrap(ctx, "owner@example.com", "pass-123", false); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.New(svc, pay, httpapi.Options{UploadDir: t.TempDir()}).Routes())
	t.Cleanup(srv.Close)
	stub.hook = srv.URL + "/api/payments/webhook"
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}, stub
}

func TestYooKassaEndToEnd(t *testing.T) {
	c, stub := setupYooKassa(t)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	// тестовой страницы оплаты нет
	if st, _, _ := c.do("POST", "/api/payments/test/"+uuid.NewString()+"/pay", nil); st != 404 && st != 405 {
		t.Fatalf("тестовая оплата при ЮKassa: %d", st)
	}

	// бронь → ссылка ЮKassa → гость оплатил → уведомление → оплачено
	st, m, _ := c.do("POST", "/api/bookings", map[string]any{"name": "Анна", "phone": "9001112233", "date": tomorrow, "time": "12:00", "guests": 2})
	if st != 201 || !strings.HasPrefix(fmt.Sprint(m["confirmation_url"]), "https://yoomoney.test/checkout/") {
		t.Fatalf("бронь: %d %v", st, m)
	}
	pid, bid := m["payment_id"].(string), fmt.Sprint(m["id"])
	yid := stub.byURL(m["confirmation_url"].(string))
	if ret := stub.payments[yid]["return_url"]; ret != "https://kofesobaken.test/?payment="+pid+"#booking" {
		t.Fatalf("return_url: %v", ret)
	}
	if code := stub.notify(payments.EventSucceeded, yid); code != 200 { // подделка: по API ещё pending
		t.Fatalf("уведомление о неоплаченном: %d", code)
	}
	if _, m, _ := c.do("GET", "/api/payments/"+pid, nil); m["status"] != "pending" {
		t.Fatalf("до оплаты: %v", m)
	}
	stub.setStatus(yid, "succeeded")
	if code := stub.notify(payments.EventSucceeded, yid); code != 200 {
		t.Fatalf("уведомление: %d", code)
	}
	if code := stub.notify(payments.EventSucceeded, yid); code != 200 {
		t.Fatalf("повторное уведомление: %d", code)
	}
	if _, m, _ := c.do("GET", "/api/payments/"+pid, nil); m["status"] != "succeeded" {
		t.Fatalf("после оплаты: %v", m)
	}
	if code := stub.notify(payments.EventSucceeded, "yk-forged"); code != 401 {
		t.Fatalf("неизвестный платёж: %d", code)
	}

	// отмена брони → возврат через ЮKassa → refund.succeeded идемпотентно
	c.do("POST", "/api/admin/login", map[string]string{"email": "owner@example.com", "password": "pass-123"})
	if st, m, _ := c.do("POST", "/api/admin/bookings/"+bid+"/cancel", nil); st != 200 || m["status"] != "cancelled" {
		t.Fatalf("отмена брони: %d %v", st, m)
	}
	if len(stub.refunds) != 1 {
		t.Fatalf("возвратов: %d", len(stub.refunds))
	}
	for rid, rf := range stub.refunds {
		if rf["payment_id"] != yid || rf["amount"].(map[string]any)["value"] != "500.00" {
			t.Fatalf("возврат: %v", rf)
		}
		if code := stub.notify(payments.EventRefunded, rid); code != 200 {
			t.Fatalf("refund.succeeded: %d", code)
		}
	}
	if _, m, _ := c.do("GET", "/api/payments/"+pid, nil); m["status"] != "refunded" {
		t.Fatalf("после возврата: %v", m)
	}

	// заказ: гость вернулся раньше уведомления — статус проверяется по API
	_, site, _ := c.do("GET", "/api/site", nil)
	first := site["menu"].([]any)[0].(map[string]any)["id"]
	st, m, _ = c.do("POST", "/api/orders", map[string]any{"mode": "pickup", "phone": "9001112233", "items": []any{map[string]any{"menu_item_id": first, "qty": 1}}})
	if st != 201 {
		t.Fatalf("заказ: %d %v", st, m)
	}
	opid := m["payment_id"].(string)
	stub.setStatus(stub.byURL(m["confirmation_url"].(string)), "succeeded")
	if _, m, _ := c.do("GET", "/api/payments/"+opid, nil); m["status"] != "succeeded" {
		t.Fatalf("заказ при возврате гостя: %v", m)
	}
	if _, _, raw := c.do("GET", "/api/admin/orders", nil); !strings.Contains(string(raw), `"status":"new"`) {
		t.Fatalf("заказ не оплачен: %s", raw)
	}

	// мероприятие: отказ от оплаты → canceled
	in := map[string]any{"title": "Встреча", "kind": "встреча", "date": tomorrow, "time": "12:00", "seats": 5, "status": "published", "price_kop": 70000}
	st, m, _ = c.do("POST", "/api/admin/events", in)
	if st != 201 {
		t.Fatalf("мероприятие: %d %v", st, m)
	}
	eid := fmt.Sprint(m["id"])
	st, m, _ = c.do("POST", "/api/events/"+eid+"/signups", map[string]any{"name": "Мария", "via": "tg", "contact": "@maria_corgi", "people": 1})
	if st != 201 || m["status"] != "awaiting_payment" {
		t.Fatalf("запись: %d %v", st, m)
	}
	spid, syid := m["payment_id"].(string), stub.byURL(m["confirmation_url"].(string))
	stub.setStatus(syid, "canceled")
	if code := stub.notify(payments.EventCanceled, syid); code != 200 {
		t.Fatalf("payment.canceled: %d", code)
	}
	if _, m, _ := c.do("GET", "/api/payments/"+spid, nil); m["status"] != "canceled" {
		t.Fatalf("после отказа: %v", m)
	}
	// вторая запись — оплачена
	st, m, _ = c.do("POST", "/api/events/"+eid+"/signups", map[string]any{"name": "Ольга", "via": "phone", "contact": "9001112244", "people": 2})
	if st != 201 {
		t.Fatalf("запись 2: %d %v", st, m)
	}
	s2pid, s2yid := m["payment_id"].(string), stub.byURL(m["confirmation_url"].(string))
	stub.setStatus(s2yid, "succeeded")
	if code := stub.notify(payments.EventSucceeded, s2yid); code != 200 {
		t.Fatalf("оплата записи: %d", code)
	}
	if _, m, _ := c.do("GET", "/api/payments/"+s2pid, nil); m["status"] != "succeeded" || m["amount_kop"].(float64) != 140000 {
		t.Fatalf("запись оплачена: %v", m)
	}

	// лишние события и мусор
	if code := stub.notify("payment.waiting_for_capture", s2yid); code != 200 {
		t.Fatalf("лишнее событие: %d", code)
	}
	res, _ := http.Post(stub.hook, "application/json", strings.NewReader("{"))
	if res.StatusCode != 400 {
		t.Fatalf("мусор: %d", res.StatusCode)
	}
}
