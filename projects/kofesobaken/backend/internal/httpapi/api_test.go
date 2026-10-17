package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kofesobaken/backend/internal/httpapi"
	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
	"kofesobaken/backend/migrations"
)

// Тесты API целиком: настоящий роутер, сервис и PostgreSQL (TEST_DATABASE_URL).

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *client) do(method, path string, body any) (int, map[string]any, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return res.StatusCode, m, raw
}

func setup(t *testing.T) (*client, *payments.TestProvider) {
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
	loc, _ := time.LoadLocation("Europe/Moscow")
	pay := payments.NewTestProvider("http://site.test", "", "whsecret")
	svc := service.New(pool, pay, []notify.Channel{notify.LogChannel{Channel_: "email"}, notify.LogChannel{Channel_: "telegram"}}, loc)
	if err := svc.Bootstrap(ctx, "owner@example.com", "pass-123", false); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.New(svc, pay, httpapi.Options{UploadDir: t.TempDir()}).Routes())
	t.Cleanup(srv.Close)
	pay.WebhookURL = srv.URL + "/api/payments/webhook"
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}, pay
}

func TestPublicAndAdminFlow(t *testing.T) {
	c, _ := setup(t)

	if st, m, _ := c.do("GET", "/api/healthz", nil); st != 200 || m["status"] != "ok" {
		t.Fatalf("healthz: %d %v", st, m)
	}
	if st, _, _ := c.do("GET", "/healthz", nil); st != 200 {
		t.Fatal("healthz без /api")
	}
	st, site, _ := c.do("GET", "/api/site", nil)
	if st != 200 || len(site["menu"].([]any)) != 9 {
		t.Fatalf("site: %d", st)
	}
	firstID := site["menu"].([]any)[0].(map[string]any)["id"]

	// кабинет закрыт
	if st, m, _ := c.do("GET", "/api/admin/bookings", nil); st != 401 || m["error"].(map[string]any)["code"] != "unauthorized" {
		t.Fatalf("без входа: %d %v", st, m)
	}
	if st, _, _ := c.do("POST", "/api/admin/login", map[string]string{"email": "owner@example.com", "password": "x"}); st != 401 {
		t.Fatalf("неверный пароль: %d", st)
	}
	if st, _, _ := c.do("POST", "/api/admin/login", map[string]string{"email": "owner@example.com", "password": "pass-123"}); st != 200 {
		t.Fatalf("вход: %d", st)
	}
	if st, m, _ := c.do("GET", "/api/admin/me", nil); st != 200 || m["email"] != "owner@example.com" {
		t.Fatalf("me: %d %v", st, m)
	}

	// заказ: ошибки полей → 422 с полями
	st, m, _ := c.do("POST", "/api/orders", map[string]any{"mode": "pickup", "phone": "1", "items": []any{}})
	if st != 422 || m["error"].(map[string]any)["fields"].(map[string]any)["phone"] == nil {
		t.Fatalf("422: %d %v", st, m)
	}
	st, m, _ = c.do("POST", "/api/orders", map[string]any{"mode": "pickup", "phone": "9001112233", "items": []any{map[string]any{"menu_item_id": firstID, "qty": 2}}})
	if st != 201 || m["confirmation_url"] == nil {
		t.Fatalf("заказ: %d %v", st, m)
	}
	pid := m["payment_id"].(string)
	if st, m, _ := c.do("POST", "/api/payments/test/"+pid+"/pay", nil); st != 200 || m["status"] != "succeeded" {
		t.Fatalf("тестовая оплата: %d %v", st, m)
	}
	if st, m, _ := c.do("GET", "/api/payments/"+pid, nil); st != 200 || m["kind"] != "order" {
		t.Fatalf("статус: %d %v", st, m)
	}
	if st, _, _ := c.do("GET", "/api/payments/not-uuid", nil); st != 404 {
		t.Fatal("кривой id платежа — 404")
	}
	st, _, raw := c.do("GET", "/api/admin/orders", nil)
	var orders []map[string]any
	_ = json.Unmarshal(raw, &orders)
	if st != 200 || len(orders) != 1 || orders[0]["status"] != "new" {
		t.Fatalf("заказы: %d %s", st, raw)
	}
	oid := fmt.Sprint(orders[0]["id"])
	if st, _, _ := c.do("POST", "/api/admin/orders/"+oid+"/done", nil); st != 200 {
		t.Fatal("готово")
	}
	if st, _, _ := c.do("POST", "/api/admin/orders/"+oid+"/cancel", nil); st != 409 {
		t.Fatal("закрытый заказ не отменить — 409")
	}
	if st, _, _ := c.do("POST", "/api/admin/orders/abc/done", nil); st != 404 {
		t.Fatal("кривой id — 404")
	}
	st, _, raw = c.do("GET", "/api/admin/notifications", nil)
	if st != 200 || !strings.Contains(string(raw), "Новый заказ") {
		t.Fatalf("журнал: %s", raw)
	}

	// бронь
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	st, m, _ = c.do("POST", "/api/bookings", map[string]any{"name": "Анна", "phone": "9001112233", "date": tomorrow, "time": "12:00", "guests": 2, "with_dog": false})
	if st != 201 || m["amount_kop"].(float64) != 50000 {
		t.Fatalf("бронь: %d %v", st, m)
	}
	c.do("POST", "/api/payments/test/"+m["payment_id"].(string)+"/pay", nil)
	bid := fmt.Sprint(m["id"])
	if st, m, _ := c.do("POST", "/api/admin/bookings/"+bid+"/confirm", nil); st != 200 || m["status"] != "confirmed" {
		t.Fatalf("подтверждение: %d %v", st, m)
	}
	if st, _, _ := c.do("POST", "/api/admin/bookings/"+bid+"/confirm", nil); st != 409 {
		t.Fatal("повторное подтверждение — 409")
	}
	if st, m, _ := c.do("POST", "/api/admin/bookings/"+bid+"/cancel", nil); st != 200 || m["status"] != "cancelled" {
		t.Fatal("отмена брони")
	}
	if st, _, raw := c.do("GET", "/api/admin/bookings", nil); st != 200 || !strings.Contains(string(raw), "Анна") {
		t.Fatal("список броней")
	}

	// меню
	st, m, _ = c.do("POST", "/api/admin/menu", map[string]any{"name": "Матча", "price_kop": 45000, "category": "other"})
	if st != 201 {
		t.Fatalf("меню: %d %v", st, m)
	}
	mid := fmt.Sprint(m["id"])
	if st, _, _ := c.do("PUT", "/api/admin/menu/"+mid, map[string]any{"name": "Матча", "price_kop": 0, "category": "other"}); st != 422 {
		t.Fatal("цена 0 — 422")
	}
	if st, _, _ := c.do("PUT", "/api/admin/menu/"+mid, map[string]any{"name": "Матча латте", "price_kop": 47000, "category": "milk"}); st != 200 {
		t.Fatal("изменение меню")
	}
	if st, _, _ := c.do("GET", "/api/admin/menu", nil); st != 200 {
		t.Fatal("меню список")
	}
	if st, _, _ := c.do("DELETE", "/api/admin/menu/"+mid, nil); st != 204 {
		t.Fatal("удаление меню")
	}
	if st, _, _ := c.do("DELETE", "/api/admin/menu/"+mid, nil); st != 404 {
		t.Fatal("повторное удаление — 404")
	}

	// мероприятия
	in := map[string]any{"title": "Встреча", "kind": "встреча", "date": tomorrow, "time": "12:00", "seats": 2, "status": "published"}
	st, m, _ = c.do("POST", "/api/admin/events", in)
	if st != 201 {
		t.Fatalf("мероприятие: %d %v", st, m)
	}
	eid := fmt.Sprint(m["id"])
	in["seats"] = 3
	if st, _, _ := c.do("PUT", "/api/admin/events/"+eid, in); st != 200 {
		t.Fatal("изменение мероприятия")
	}
	st, m, _ = c.do("POST", "/api/events/"+eid+"/signups", map[string]any{"name": "Мария", "via": "tg", "contact": "@maria_corgi", "people": 3, "with_dog": true})
	if st != 201 || m["status"] != "confirmed" {
		t.Fatalf("запись: %d %v", st, m)
	}
	if st, m, _ := c.do("POST", "/api/events/"+eid+"/signups", map[string]any{"name": "Ещё", "via": "tg", "contact": "@another", "people": 1}); st != 409 {
		t.Fatalf("мест нет — 409: %d %v", st, m)
	}
	if st, _, raw := c.do("GET", "/api/admin/events/"+eid+"/signups", nil); st != 200 || !strings.Contains(string(raw), "@maria_corgi") {
		t.Fatal("записавшиеся")
	}
	if st, _, _ := c.do("GET", "/api/admin/events", nil); st != 200 {
		t.Fatal("список мероприятий")
	}
	if st, _, _ := c.do("POST", "/api/events/9999/signups", map[string]any{"name": "x", "via": "tg", "contact": "@xxxx", "people": 1}); st != 404 {
		t.Fatal("нет мероприятия — 404")
	}
	if st, _, _ := c.do("DELETE", "/api/admin/events/"+eid, nil); st != 204 {
		t.Fatal("удаление мероприятия")
	}

	// тексты и настройки
	st, content, raw := c.do("GET", "/api/admin/content", nil)
	if st != 200 {
		t.Fatal("тексты")
	}
	content["hero_chip"] = "Новая подпись"
	if st, m, _ := c.do("PUT", "/api/admin/content", content); st != 200 || m["hero_chip"] != "Новая подпись" {
		t.Fatalf("сохранение текстов: %d %v %s", st, m, raw)
	}
	st, s, _ := c.do("GET", "/api/admin/settings", nil)
	if st != 200 || s["delivery_min_kop"].(float64) != 100000 {
		t.Fatalf("настройки: %v", s)
	}
	s["delivery_min_kop"] = 120000
	if st, m, _ := c.do("PUT", "/api/admin/settings", s); st != 200 || m["delivery_min_kop"].(float64) != 120000 {
		t.Fatal("сохранение настроек")
	}
	if st, _, _ := c.do("POST", "/api/admin/notify/test", map[string]string{"channel": "email"}); st != 200 {
		t.Fatal("проверочное письмо")
	}
	if st, m, _ := c.do("POST", "/api/admin/telegram/connect", nil); st != 200 || !m["notify"].(map[string]any)["telegram_connected"].(bool) {
		t.Fatal("подключение Telegram")
	}
	if st, _, _ := c.do("POST", "/api/admin/telegram/disconnect", nil); st != 200 {
		t.Fatal("отключение Telegram")
	}

	// выход
	if st, _, _ := c.do("POST", "/api/admin/logout", nil); st != 204 {
		t.Fatal("выход")
	}
	if st, _, _ := c.do("GET", "/api/admin/me", nil); st != 401 {
		t.Fatal("после выхода — 401")
	}
	if st, _, _ := c.do("GET", "/api/nope", nil); st != 404 {
		t.Fatal("нет такого пути — 404 JSON")
	}
}

func TestWebhookSignatureAndBadInput(t *testing.T) {
	c, pay := setup(t)
	body := []byte(`{"event":"payment.succeeded","payment_id":"00000000-0000-0000-0000-000000000000","provider_payment_id":"x","amount_kop":1}`)
	req, _ := http.NewRequest("POST", c.base+"/api/payments/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature", "bad")
	res, _ := c.http.Do(req)
	if res.StatusCode != 401 {
		t.Fatalf("неверная подпись — 401, а не %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", c.base+"/api/payments/webhook", bytes.NewReader([]byte("{")))
	req.Header.Set("X-Signature", payments.Sign(pay.Secret, []byte("{")))
	res, _ = c.http.Do(req)
	if res.StatusCode != 400 {
		t.Fatalf("кривое уведомление — 400, а не %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", c.base+"/api/payments/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature", payments.Sign(pay.Secret, body))
	res, _ = c.http.Do(req)
	if res.StatusCode != 404 {
		t.Fatalf("нет платежа — 404, а не %d", res.StatusCode)
	}
	// не JSON — 415
	req, _ = http.NewRequest("POST", c.base+"/api/bookings", strings.NewReader("name=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, _ = c.http.Do(req)
	if res.StatusCode != 415 {
		t.Fatalf("форма не JSON — 415, а не %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", c.base+"/api/bookings", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	res, _ = c.http.Do(req)
	if res.StatusCode != 400 {
		t.Fatalf("битый JSON — 400, а не %d", res.StatusCode)
	}
}

func TestUpload(t *testing.T) {
	c, _ := setup(t)
	c.do("POST", "/api/admin/login", map[string]string{"email": "owner@example.com", "password": "pass-123"})
	send := func(name string, data []byte) (int, map[string]any) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, _ := w.CreateFormFile("file", name)
		fw.Write(data)
		w.Close()
		req, _ := http.NewRequest("POST", c.base+"/api/admin/uploads", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		res, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m
	}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde")
	st, m := send("dog.png", png)
	if st != 201 || !strings.HasSuffix(m["url"].(string), ".png") {
		t.Fatalf("загрузка фото: %d %v", st, m)
	}
	res, _ := c.http.Get(c.base + m["url"].(string))
	if res.StatusCode != 200 {
		t.Fatalf("фото отдаётся: %d", res.StatusCode)
	}
	if st, _ := send("evil.html", []byte("<html><script>alert(1)</script>")); st != 422 {
		t.Fatalf("не фото — 422, а не %d", st)
	}
	res, _ = c.http.Get(c.base + "/uploads/")
	if res.StatusCode != 404 {
		t.Fatal("список файлов не показываем")
	}
}

func TestRateLimiter(t *testing.T) {
	l := httpapi.NewRateLimiter(2, time.Minute)
	if !l.Allow("a") || !l.Allow("a") || l.Allow("a") {
		t.Fatal("третья попытка за минуту — отказ")
	}
	if !l.Allow("b") {
		t.Fatal("другой адрес не ограничен")
	}
	if !l.Blocked("a") || l.Blocked("b") || l.Blocked("c") {
		t.Fatal("Blocked")
	}
	l.Fail("c")
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req.RemoteAddr = "a"
	h.ServeHTTP(rr, req)
	if rr.Code != 429 {
		t.Fatalf("ждали 429, получили %d", rr.Code)
	}
}
