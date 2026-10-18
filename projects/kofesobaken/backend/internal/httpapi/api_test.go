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
	"kofesobaken/backend/internal/media"
	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
	"kofesobaken/backend/internal/store"
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
	c, pay, _ := setupWith(t, nil)
	return c, pay
}

// setupWith — сервер с выбранным хранилищем фото (nil — диск во временной папке); pool — чтобы «перезапустить» сервер.
func setupWith(t *testing.T, photos func(*pgxpool.Pool) media.Store) (*client, *payments.TestProvider, *pgxpool.Pool) {
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
	o := httpapi.Options{UploadDir: t.TempDir()}
	if photos != nil {
		o.Media = photos(pool)
	}
	srv := httptest.NewServer(httpapi.New(svc, pay, o).Routes())
	t.Cleanup(srv.Close)
	pay.WebhookURL = srv.URL + "/api/payments/webhook"
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}, pay, pool
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
	testUpload(t, c)
}

// Фото в PostgreSQL (хостинг без диска): после «перезапуска» — новый сервер на той же базе — фото на месте.
func TestUploadPostgresSurvivesRestart(t *testing.T) {
	pg := func(p *pgxpool.Pool) media.Store { return media.Postgres{Q: store.New(p)} }
	c, pay, pool := setupWith(t, pg)
	url := testUpload(t, c)
	loc, _ := time.LoadLocation("Europe/Moscow")
	svc := service.New(pool, pay, nil, loc)
	srv2 := httptest.NewServer(httpapi.New(svc, pay, httpapi.Options{Media: pg(pool)}).Routes())
	defer srv2.Close()
	res, err := http.Get(srv2.URL + url)
	if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("после перезапуска: %v %v", err, res)
	}
	if res, _ := http.Get(srv2.URL + "/uploads/20260101-000000000000000000000000.png"); res.StatusCode != 404 {
		t.Fatalf("нет такого фото: %d", res.StatusCode)
	}
	if res, _ := http.Get(srv2.URL + "/uploads/..%2f..%2fetc%2fpasswd"); res.StatusCode != 404 {
		t.Fatalf("чужой путь: %d", res.StatusCode)
	}
}

func testUpload(t *testing.T, c *client) string {
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
	return m["url"].(string)
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

// newClient — второй человек со своей cookie (сотрудник), тот же сервер.
func (c *client) newClient() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: c.t, base: c.base, http: &http.Client{Jar: jar}}
}

func errCode(m map[string]any) any {
	if e, ok := m["error"].(map[string]any); ok {
		return e["code"]
	}
	return nil
}

func TestClosedWithoutLogin(t *testing.T) {
	c, _ := setup(t)
	for _, p := range [][2]string{
		{"GET", "/api/admin/me"}, {"GET", "/api/admin/bookings"}, {"POST", "/api/admin/bookings/1/confirm"},
		{"GET", "/api/admin/orders"}, {"GET", "/api/admin/menu"}, {"POST", "/api/admin/menu"}, {"DELETE", "/api/admin/menu/1"},
		{"GET", "/api/admin/events"}, {"GET", "/api/admin/events/1/signups"}, {"GET", "/api/admin/content"}, {"PUT", "/api/admin/content"},
		{"GET", "/api/admin/settings"}, {"GET", "/api/admin/notifications"}, {"POST", "/api/admin/uploads"},
		{"GET", "/api/admin/staff"}, {"POST", "/api/admin/staff"}, {"PUT", "/api/admin/staff/1"}, {"POST", "/api/admin/staff/1/password"},
		{"POST", "/api/admin/password"}, {"GET", "/api/admin/questions"}, {"POST", "/api/admin/questions/1/answer"},
	} {
		st, m, _ := c.do(p[0], p[1], map[string]any{})
		if st != 401 || errCode(m) != "unauthorized" {
			t.Errorf("%s %s без входа: %d %v", p[0], p[1], st, m)
		}
	}
	// поддельная cookie
	req, _ := http.NewRequest("GET", c.base+"/api/admin/bookings", nil)
	req.AddCookie(&http.Cookie{Name: "ks_session", Value: "подделка"})
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Fatalf("поддельная cookie: %d", res.StatusCode)
	}
}

func TestStaffRoles(t *testing.T) {
	admin, _ := setup(t)
	if st, m, _ := admin.do("POST", "/api/admin/login", map[string]string{"email": "owner@example.com", "password": "pass-123"}); st != 200 || m["role"] != "admin" {
		t.Fatalf("вход администратора: %d %v", st, m)
	}
	// создать сотрудника
	if st, m, _ := admin.do("POST", "/api/admin/staff", map[string]string{"name": "", "email": "x", "password": "1"}); st != 422 || len(m["error"].(map[string]any)["fields"].(map[string]any)) != 3 {
		t.Fatalf("ошибки полей: %d %v", st, m)
	}
	st, m, _ := admin.do("POST", "/api/admin/staff", map[string]string{"name": "Анна", "email": "anna@example.com", "password": "temp-pass-1"})
	if st != 201 || m["role"] != "staff" || m["must_change_password"] != true || m["password_hash"] != nil {
		t.Fatalf("создать: %d %v", st, m)
	}
	id := fmt.Sprint(m["id"])
	if st, _, _ := admin.do("POST", "/api/admin/staff", map[string]string{"name": "Анна", "email": "anna@example.com", "password": "temp-pass-1"}); st != 409 {
		t.Fatalf("та же почта: %d", st)
	}

	// сотрудник входит с временным паролем: до смены пароля — только /me и /password
	staff := admin.newClient()
	st, m, _ = staff.do("POST", "/api/admin/login", map[string]string{"email": "anna@example.com", "password": "temp-pass-1"})
	if st != 200 || m["role"] != "staff" || m["must_change_password"] != true {
		t.Fatalf("вход сотрудника: %d %v", st, m)
	}
	if st, m, _ := staff.do("GET", "/api/admin/bookings", nil); st != 403 || errCode(m) != "password_change_required" {
		t.Fatalf("с временным паролем: %d %v", st, m)
	}
	if st, m, _ := staff.do("POST", "/api/admin/password", map[string]string{"new_password": "short"}); st != 422 || m["error"].(map[string]any)["fields"].(map[string]any)["new_password"] == nil {
		t.Fatalf("короткий пароль: %d %v", st, m)
	}
	if st, m, _ := staff.do("POST", "/api/admin/password", map[string]string{"new_password": "anna-own-1"}); st != 200 || m["must_change_password"] != false {
		t.Fatalf("смена временного пароля: %d %v", st, m)
	}
	if st, m, _ := staff.do("POST", "/api/admin/password", map[string]string{"current_password": "nope", "new_password": "anna-own-2"}); st != 403 || errCode(m) != "wrong_password" || m["error"].(map[string]any)["fields"].(map[string]any)["current_password"] == nil {
		t.Fatalf("неверный текущий пароль: %d %v", st, m)
	}

	// сотруднику — брони, заказы, меню, мероприятия; админские разделы — 403
	for _, p := range []string{"/api/admin/bookings", "/api/admin/orders", "/api/admin/menu", "/api/admin/events"} {
		if st, _, _ := staff.do("GET", p, nil); st != 200 {
			t.Errorf("сотрудник %s: %d", p, st)
		}
	}
	for _, p := range [][2]string{
		{"GET", "/api/admin/content"}, {"PUT", "/api/admin/content"}, {"GET", "/api/admin/settings"}, {"PUT", "/api/admin/settings"},
		{"POST", "/api/admin/notify/test"}, {"POST", "/api/admin/telegram/connect"}, {"POST", "/api/admin/telegram/disconnect"},
		{"GET", "/api/admin/notifications"}, {"GET", "/api/admin/staff"}, {"POST", "/api/admin/staff"},
		{"PUT", "/api/admin/staff/" + id}, {"POST", "/api/admin/staff/" + id + "/password"},
	} {
		st, m, _ := staff.do(p[0], p[1], map[string]any{"name": "x", "email": "y@example.com", "password": "temp-pass-1", "active": true})
		if st != 403 || errCode(m) != "forbidden" {
			t.Errorf("сотрудник %s %s: %d %v", p[0], p[1], st, m)
		}
	}
	if st, m, _ := staff.do("GET", "/api/admin/me", nil); st != 200 || m["name"] != "Анна" || m["role"] != "staff" {
		t.Fatalf("me сотрудника: %d %v", st, m)
	}

	// список и отключение: отключённый сразу теряет доступ и не может войти
	if st, _, raw := admin.do("GET", "/api/admin/staff", nil); st != 200 || !strings.Contains(string(raw), "anna@example.com") || strings.Contains(string(raw), "password_hash") {
		t.Fatalf("список: %d %s", st, raw)
	}
	if st, m, _ := admin.do("PUT", "/api/admin/staff/"+id, map[string]any{"name": "Анна", "email": "anna@example.com", "active": false}); st != 200 || m["active"] != false {
		t.Fatalf("отключить: %d %v", st, m)
	}
	if st, _, _ := staff.do("GET", "/api/admin/bookings", nil); st != 401 {
		t.Fatalf("отключённый: %d", st)
	}
	if st, _, _ := staff.do("POST", "/api/admin/login", map[string]string{"email": "anna@example.com", "password": "anna-own-1"}); st != 401 {
		t.Fatalf("отключённый входит: %d", st)
	}
	// включить и сбросить пароль
	admin.do("PUT", "/api/admin/staff/"+id, map[string]any{"name": "Анна", "email": "anna@example.com", "active": true})
	if st, m, _ := admin.do("POST", "/api/admin/staff/"+id+"/password", map[string]string{"password": "temp-pass-2"}); st != 200 || m["must_change_password"] != true {
		t.Fatalf("сброс пароля: %d %v", st, m)
	}
	if st, m, _ := staff.do("POST", "/api/admin/login", map[string]string{"email": "anna@example.com", "password": "temp-pass-2"}); st != 200 || m["must_change_password"] != true {
		t.Fatalf("вход после сброса: %d %v", st, m)
	}
	// себя не отключить
	_, me, _ := admin.do("GET", "/api/admin/me", nil)
	if st, _, _ := admin.do("PUT", fmt.Sprintf("/api/admin/staff/%v", me["id"]), map[string]any{"name": "x", "email": "owner@example.com", "active": false}); st != 409 {
		t.Fatalf("отключить себя: %d", st)
	}
	if st, _, _ := admin.do("GET", "/api/admin/me", nil); st != 200 {
		t.Fatal("администратор на месте")
	}
}

// «Профиль»: смена своего пароля и почты; неверный текущий пароль — 403, занятая почта — 409,
// другие сессии закрываются, текущая остаётся.
func TestProfile(t *testing.T) {
	admin, _ := setup(t)
	login := func(c *client, email, pw string) int {
		st, _, _ := c.do("POST", "/api/admin/login", map[string]string{"email": email, "password": pw})
		return st
	}
	if login(admin, "owner@example.com", "pass-123") != 200 {
		t.Fatal("вход")
	}
	other := admin.newClient() // тот же человек на другом устройстве
	if login(other, "owner@example.com", "pass-123") != 200 {
		t.Fatal("вход со второго устройства")
	}
	admin.do("POST", "/api/admin/staff", map[string]string{"name": "Анна", "email": "anna@example.com", "password": "temp-pass-1"})

	// пароль
	if st, m, _ := admin.do("POST", "/api/admin/password", map[string]string{"current_password": "wrong-1", "new_password": "owner-new-1"}); st != 403 || errCode(m) != "wrong_password" {
		t.Fatalf("неверный текущий пароль: %d %v", st, m)
	}
	if st, _, _ := admin.do("POST", "/api/admin/password", map[string]string{"current_password": "pass-123", "new_password": "short"}); st != 422 {
		t.Fatalf("короткий новый пароль: %d", st)
	}
	if st, m, _ := admin.do("POST", "/api/admin/password", map[string]string{"current_password": "pass-123", "new_password": "owner-new-1"}); st != 200 || m["email"] != "owner@example.com" {
		t.Fatalf("смена пароля: %d %v", st, m)
	}
	if st, _, _ := other.do("GET", "/api/admin/me", nil); st != 401 {
		t.Fatalf("другая сессия после смены пароля: %d", st)
	}
	if st, _, _ := admin.do("GET", "/api/admin/me", nil); st != 200 {
		t.Fatalf("своя сессия после смены пароля: %d", st)
	}
	if login(other, "owner@example.com", "pass-123") != 401 || login(other, "owner@example.com", "owner-new-1") != 200 {
		t.Fatal("вход: старый пароль не подходит, новый — да")
	}

	// почта
	for _, c := range []struct {
		body map[string]string
		st   int
		code string
	}{
		{map[string]string{"current_password": "wrong-2", "email": "boss@example.com"}, 403, "wrong_password"},
		{map[string]string{"current_password": "owner-new-1", "email": "Anna@Example.com"}, 409, "conflict"},
		{map[string]string{"current_password": "owner-new-1", "email": "not-an-email"}, 422, "validation"},
		{map[string]string{"current_password": "owner-new-1", "email": "owner@example.com"}, 422, "validation"},
	} {
		if st, m, _ := admin.do("POST", "/api/admin/email", c.body); st != c.st || errCode(m) != c.code {
			t.Errorf("почта %v: %d %v", c.body, st, m)
		}
	}
	if st, m, _ := admin.do("POST", "/api/admin/email", map[string]string{"current_password": "owner-new-1", "email": " Boss@Example.com "}); st != 200 || m["email"] != "boss@example.com" || m["role"] != "admin" {
		t.Fatalf("смена почты: %d %v", st, m)
	}
	if st, _, _ := other.do("GET", "/api/admin/me", nil); st != 401 {
		t.Fatalf("другая сессия после смены почты: %d", st)
	}
	if st, m, _ := admin.do("GET", "/api/admin/me", nil); st != 200 || m["email"] != "boss@example.com" {
		t.Fatalf("своя сессия после смены почты: %d %v", st, m)
	}
	if login(other, "owner@example.com", "owner-new-1") != 401 || login(other, "boss@example.com", "owner-new-1") != 200 {
		t.Fatal("вход: старая почта не подходит, новая — да")
	}

	// сотрудник тоже может; с временным паролем — сначала смена пароля
	staff := admin.newClient()
	login(staff, "anna@example.com", "temp-pass-1")
	if st, m, _ := staff.do("POST", "/api/admin/email", map[string]string{"current_password": "temp-pass-1", "email": "anna2@example.com"}); st != 403 || errCode(m) != "password_change_required" {
		t.Fatalf("почта с временным паролем: %d %v", st, m)
	}
	staff.do("POST", "/api/admin/password", map[string]string{"new_password": "anna-own-1"})
	if st, m, _ := staff.do("POST", "/api/admin/email", map[string]string{"current_password": "anna-own-1", "email": "anna2@example.com"}); st != 200 || m["role"] != "staff" {
		t.Fatalf("почта сотрудника: %d %v", st, m)
	}

	// неверный текущий пароль ограничен как вход: после 8 за 15 минут — 429
	for i := 0; i < 8; i++ {
		admin.do("POST", "/api/admin/email", map[string]string{"current_password": "guess", "email": "x@example.com"})
	}
	if st, m, _ := admin.do("POST", "/api/admin/email", map[string]string{"current_password": "owner-new-1", "email": "x@example.com"}); st != 429 || errCode(m) != "rate_limited" {
		t.Fatalf("ограничение частоты: %d %v", st, m)
	}
}

func TestSetupEndpoint(t *testing.T) {
	c, _ := setup(t)
	if st, m, _ := c.do("POST", "/api/admin/setup", map[string]string{"token": "нет-такого", "password": "boss-pass-1"}); st != 410 || errCode(m) != "setup_expired" {
		t.Fatalf("неверная ссылка: %d %v", st, m)
	}
	if st, _, _ := c.do("POST", "/api/admin/setup/info", map[string]string{"token": ""}); st != 410 {
		t.Fatalf("пустая ссылка: %d", st)
	}
}

func TestForgotAndCall(t *testing.T) {
	c, _ := setup(t)
	// почта не настроена (заглушка) — mail=false, кабинет подскажет обратиться к администратору
	if st, m, _ := c.do("POST", "/api/admin/forgot", map[string]string{"email": "owner@example.com"}); st != 200 || m["mail"] != false {
		t.Fatalf("забыли пароль: %d %v", st, m)
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedirect.Get(c.base + "/api/call/+79000000001")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "tel:+79000000001" {
		t.Fatalf("позвонить: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	for _, bad := range []string{"javascript:alert(1)", "https:%2F%2Fevil.test", "12"} {
		res, err := noRedirect.Get(c.base + "/api/call/" + bad)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("позвонить %q: %d", bad, res.StatusCode)
		}
	}
}

func TestLoginLimitNotBypassedByHeaders(t *testing.T) {
	c, _ := setup(t)
	try := func(hdr, val, email string) int {
		b, _ := json.Marshal(map[string]string{"email": email, "password": "wrong"})
		req, _ := http.NewRequest("POST", c.base+"/api/admin/login", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if hdr != "" {
			req.Header.Set(hdr, val)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	// подбор пароля с подменой True-Client-IP / X-Forwarded-For: лимит по адресу всё равно срабатывает
	for i := 0; i < 8; i++ {
		try("True-Client-IP", fmt.Sprintf("10.0.0.%d", i), "owner@example.com")
	}
	if st := try("X-Forwarded-For", "10.9.9.9", "owner@example.com"); st != 429 {
		t.Fatalf("ждали 429 при подмене заголовков, получили %d", st)
	}
	// лимит на почту — с разных адресов (X-Real-IP ставит наш nginx)
	for i := 0; i < 20; i++ {
		try("X-Real-IP", fmt.Sprintf("10.1.0.%d", i), "staff@example.com")
	}
	if st := try("X-Real-IP", "10.2.0.1", "staff@example.com"); st != 429 {
		t.Fatalf("ждали 429 по почте, получили %d", st)
	}
	if st := try("X-Real-IP", "10.2.0.2", "other@example.com"); st != 401 {
		t.Fatalf("другая почта с нового адреса: %d", st)
	}
}

func TestQuestionsAPI(t *testing.T) {
	c, _ := setup(t)
	// ошибки полей — 422 с полями
	st, m, _ := c.do("POST", "/api/questions", map[string]any{"name": "Анна", "via": "email", "contact": "нет", "question": "Можно с котом?"})
	if st != 422 || m["error"].(map[string]any)["fields"].(map[string]any)["contact"] == nil {
		t.Fatalf("422: %d %v", st, m)
	}
	if st, _, _ := c.do("POST", "/api/questions", map[string]any{"name": "Анна", "via": "email", "contact": "anna@example.com", "question": "Можно с котом?"}); st != 201 {
		t.Fatalf("вопрос: %d", st)
	}
	if st, _, _ := c.do("POST", "/api/questions", map[string]any{"name": "Олег", "via": "telegram", "contact": "@oleg_k", "question": "Есть розетки?"}); st != 201 {
		t.Fatalf("вопрос с ником: %d", st)
	}
	// ограничение частоты: 5 в час с одного адреса (ошибочные тоже считаются)
	for i := 0; i < 2; i++ {
		c.do("POST", "/api/questions", map[string]any{"name": "Спам", "via": "email", "contact": "s@example.com", "question": "спам спам"})
	}
	if st, m, _ := c.do("POST", "/api/questions", map[string]any{"name": "Спам", "via": "email", "contact": "s@example.com", "question": "спам спам"}); st != 429 || errCode(m) != "rate_limited" {
		t.Fatalf("частота: %d %v", st, m)
	}

	// кабинет: список (новые сверху) и ответ
	if st, _, _ := c.do("POST", "/api/admin/login", map[string]string{"email": "owner@example.com", "password": "pass-123"}); st != 200 {
		t.Fatal("вход")
	}
	st, m, _ = c.do("GET", "/api/admin/questions", nil)
	items, _ := m["items"].([]any)
	if st != 200 || len(items) != 4 || m["mail_direct"] != false {
		t.Fatalf("список: %d %v", st, m)
	}
	var annaID any
	for _, it := range items {
		if q := it.(map[string]any); q["contact"] == "anna@example.com" {
			annaID = q["id"]
			if q["can_send"] != false || q["answered_at"] != nil {
				t.Fatalf("вопрос: %v", q)
			}
		}
	}
	path := fmt.Sprintf("/api/admin/questions/%v/answer", annaID)
	if st, m, _ := c.do("POST", path, map[string]any{"answer": "Да", "send": true}); st != 409 || errCode(m) != "conflict" {
		t.Fatalf("письмом без домена: %d %v", st, m)
	}
	st, m, _ = c.do("POST", path, map[string]any{"answer": "Да, ждём", "send": false})
	if st != 200 || m["answered_via"] != "manual" || m["answer"] != "Да, ждём" || m["answered_at"] == nil {
		t.Fatalf("отметка: %d %v", st, m)
	}
	if st, _, _ := c.do("POST", "/api/admin/questions/99999/answer", map[string]any{"answer": "x"}); st != 404 {
		t.Fatalf("нет вопроса: %d", st)
	}
}
