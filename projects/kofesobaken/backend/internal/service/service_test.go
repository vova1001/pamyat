package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
	"kofesobaken/backend/migrations"
)

// Интеграционные тесты на настоящей PostgreSQL: TEST_DATABASE_URL (make test поднимает базу в Docker).

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL не задан — интеграционные тесты пропущены")
		os.Exit(m.Run())
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		panic(err)
	}
	testPool = pool
	os.Exit(m.Run())
}

var msk, _ = time.LoadLocation("Europe/Moscow")

// fakeChannel запоминает отправленные сообщения; может «падать».
type fakeChannel struct {
	name string
	sent []notify.Message
	fail bool
}

func (f *fakeChannel) Name() string { return f.name }
func (f *fakeChannel) Send(_ context.Context, _ string, m notify.Message) error {
	if f.fail {
		return errors.New("канал недоступен")
	}
	f.sent = append(f.sent, m)
	return nil
}

type env struct {
	svc   *service.Service
	pay   *payments.TestProvider
	email *fakeChannel
	tg    *fakeChannel
	hook  *httptest.Server
}

// newEnv — чистая схема, миграции, подготовка; тестовая оплата шлёт уведомления в этот же сервис.
func newEnv(t *testing.T) *env {
	t.Helper()
	if testPool == nil {
		t.Skip("нужна TEST_DATABASE_URL")
	}
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(testPool); err != nil {
		t.Fatal(err)
	}
	e := &env{email: &fakeChannel{name: "email"}, tg: &fakeChannel{name: "telegram"}}
	e.pay = payments.NewTestProvider("http://site.test", "", "secret")
	e.svc = service.New(testPool, e.pay, []notify.Channel{e.email, e.tg}, msk)
	e.hook = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ev, err := e.pay.ParseWebhook(r.Header, body)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := e.svc.HandlePaymentEvent(r.Context(), ev); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(e.hook.Close)
	e.pay.WebhookURL = e.hook.URL
	if err := e.svc.Bootstrap(ctx, "Owner@Example.com", "secret-pass", false); err != nil {
		t.Fatal(err)
	}
	return e
}

func day(n int) string { return time.Now().In(msk).AddDate(0, 0, n).Format("2006-01-02") }

func fieldErr(t *testing.T, err error, field string) {
	t.Helper()
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ждали ошибку поля %q, получили %v", field, err)
	}
	if _, ok := ve.Fields[field]; !ok {
		t.Fatalf("ждали ошибку поля %q, получили %v", field, ve.Fields)
	}
}

func conflict(t *testing.T, err error) {
	t.Helper()
	var ce *service.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("ждали конфликт, получили %v", err)
	}
}

func TestBootstrapIdempotent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.Bootstrap(ctx, "owner@example.com", "secret-pass", true); err != nil {
		t.Fatal(err)
	}
	menu, _ := e.svc.ListMenu(ctx)
	if len(menu) != 9 {
		t.Fatalf("меню из эскиза: ждали 9 позиций, есть %d", len(menu))
	}
	evs, _ := e.svc.ListEvents(ctx)
	if len(evs) != 3 {
		t.Fatalf("примеры мероприятий: ждали 3, есть %d", len(evs))
	}
	if err := e.svc.Bootstrap(ctx, "owner@example.com", "", true); err != nil {
		t.Fatal(err)
	}
	menu, _ = e.svc.ListMenu(ctx)
	evs, _ = e.svc.ListEvents(ctx)
	if len(menu) != 9 || len(evs) != 3 {
		t.Fatal("повторный запуск не должен ничего добавлять")
	}
	// Администратор без пароля: входа по паролю нет, только одноразовая ссылка.
	if err := e.svc.Bootstrap(ctx, "second@example.com", "", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, "second@example.com", ""); !errors.Is(err, service.ErrBadLogin) {
		t.Fatal("пустой пароль не должен подходить")
	}
	code, err := e.svc.PendingSetup(ctx, "second@example.com")
	if err != nil || code == "" {
		t.Fatalf("ссылка первого входа: %q %v", code, err)
	}
	// Пароль из окружения не перезаписывает уже заданный.
	if err := e.svc.Bootstrap(ctx, "owner@example.com", "other-pass-1", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, "owner@example.com", "secret-pass"); err != nil {
		t.Fatalf("прежний пароль должен остаться: %v", err)
	}
	if code, _ := e.svc.PendingSetup(ctx, "owner@example.com"); code != "" {
		t.Fatal("пароль задан — ссылка не нужна")
	}
}

func TestLoginSession(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, _, err := e.svc.Login(ctx, "owner@example.com", "wrong"); !errors.Is(err, service.ErrBadLogin) {
		t.Fatalf("неверный пароль: %v", err)
	}
	if _, _, err := e.svc.Login(ctx, "nobody@example.com", "secret-pass"); !errors.Is(err, service.ErrBadLogin) {
		t.Fatalf("нет такой почты: %v", err)
	}
	tok, acc, err := e.svc.Login(ctx, " OWNER@example.com ", "secret-pass")
	if err != nil || acc.Email != "owner@example.com" || acc.Role != service.RoleAdmin {
		t.Fatalf("вход: %v %+v", err, acc)
	}
	if got, err := e.svc.Session(ctx, tok); err != nil || got.Email != "owner@example.com" || !got.IsAdmin() {
		t.Fatalf("сессия: %v %+v", err, got)
	}
	if _, err := e.svc.Session(ctx, "bad"); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("чужой токен должен давать ErrUnauthorized")
	}
	if _, err := e.svc.Session(ctx, ""); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("пустой токен должен давать ErrUnauthorized")
	}
	if err := e.svc.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Logout(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Session(ctx, tok); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("после выхода сессия не действует")
	}
}

func TestAdminPasswordTemporary(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.svc.SetAdminPasswordTemporary(true)
	if err := e.svc.Bootstrap(ctx, "render@example.com", "generated-by-host-123", false); err != nil {
		t.Fatal(err)
	}
	tok, acc, err := e.svc.Login(ctx, "render@example.com", "generated-by-host-123")
	if err != nil || !acc.MustChangePassword {
		t.Fatalf("сгенерированный пароль — временный: %v %+v", err, acc)
	}
	if err := e.svc.ChangePassword(ctx, acc, tok, "", "owner-own-1"); err != nil {
		t.Fatal(err)
	}
	// перезапуск с тем же ADMIN_PASSWORD не возвращает временный пароль
	if err := e.svc.Bootstrap(ctx, "render@example.com", "generated-by-host-123", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, "render@example.com", "generated-by-host-123"); !errors.Is(err, service.ErrBadLogin) {
		t.Fatal("сгенерированный пароль после смены не действует")
	}
	if _, acc, err := e.svc.Login(ctx, "render@example.com", "owner-own-1"); err != nil || acc.MustChangePassword {
		t.Fatalf("свой пароль: %v %+v", err, acc)
	}
}

func TestSetupLink(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.Bootstrap(ctx, "boss@example.com", "", false); err != nil {
		t.Fatal(err)
	}
	old, _ := e.svc.PendingSetup(ctx, "boss@example.com")
	if old == "" {
		t.Fatal("первая ссылка")
	}
	// перезапуск (выкладка) не перевыпускает ссылку: отправленная заказчику продолжает работать
	if again, err := e.svc.PendingSetup(ctx, "boss@example.com"); err != nil || again != "" {
		t.Fatalf("перезапуск перевыпустил ссылку: %q %v", again, err)
	}
	if em, err := e.svc.SetupEmail(ctx, old); err != nil || em != "boss@example.com" {
		t.Fatalf("ссылка после перезапуска: %q %v", em, err)
	}
	// срок вышел — при старте новая ссылка, старая не действует
	e.svc.SetClock(func() time.Time { return time.Now().Add(service.SetupTTL + time.Minute) })
	code, _ := e.svc.PendingSetup(ctx, "boss@example.com")
	e.svc.SetClock(time.Now)
	if code == "" || code == old {
		t.Fatal("после срока — новая ссылка")
	}
	if _, _, err := e.svc.SetupPassword(ctx, old, "boss-pass-1"); !errors.Is(err, service.ErrSetupExpired) {
		t.Fatalf("старая ссылка: %v", err)
	}
	if em, err := e.svc.SetupEmail(ctx, code); err != nil || em != "boss@example.com" {
		t.Fatalf("чья ссылка: %q %v", em, err)
	}
	if _, err := e.svc.SetupEmail(ctx, old); !errors.Is(err, service.ErrSetupExpired) {
		t.Fatal("старая ссылка не действует")
	}
	if _, _, err := e.svc.SetupPassword(ctx, code, "short"); err == nil {
		t.Fatal("короткий пароль")
	} else {
		fieldErr(t, err, "password")
	}
	tok, acc, err := e.svc.SetupPassword(ctx, code, "boss-pass-1")
	if err != nil || acc.Role != service.RoleAdmin || acc.MustChangePassword {
		t.Fatalf("первый вход: %v %+v", err, acc)
	}
	if _, err := e.svc.Session(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.SetupPassword(ctx, code, "boss-pass-2"); !errors.Is(err, service.ErrSetupExpired) {
		t.Fatal("ссылка одноразовая")
	}
	if _, _, err := e.svc.Login(ctx, "boss@example.com", "boss-pass-1"); err != nil {
		t.Fatalf("вход своим паролем: %v", err)
	}
	if _, _, err := e.svc.SetupPassword(ctx, "", "boss-pass-2"); !errors.Is(err, service.ErrSetupExpired) {
		t.Fatal("пустой код")
	}
	// Забыл пароль: новая ссылка от студии (server admin-link) задаёт пароль заново и закрывает старые сессии.
	code, err = e.svc.IssueSetupLink(ctx, "boss@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.SetupPassword(ctx, code, "boss-pass-3"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Session(ctx, tok); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("старые сессии закрыты")
	}
	if _, err := e.svc.IssueSetupLink(ctx, "nobody@example.com"); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("нет такого администратора")
	}
}

func TestStaff(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreateStaff(ctx, service.StaffCreate{Name: " ", Email: "bad", Password: "123"})
	fieldErr(t, err, "name")
	fieldErr(t, err, "email")
	fieldErr(t, err, "password")
	_, owner, _ := e.svc.Login(ctx, "owner@example.com", "secret-pass")
	_, err = e.svc.CreateStaff(ctx, service.StaffCreate{Name: "Икс", Email: "x@example.com", Password: "temp-pass-1", Role: "root"})
	fieldErr(t, err, "role")
	st, err := e.svc.CreateStaff(ctx, service.StaffCreate{Name: " Анна ", Email: "Anna@Example.com", Password: "temp-pass-1"})
	if err != nil || st.Name != "Анна" || st.Email != "anna@example.com" || st.Role != service.RoleStaff || !st.Active || !st.MustChangePassword {
		t.Fatalf("создать: %v %+v", err, st)
	}
	_, err = e.svc.CreateStaff(ctx, service.StaffCreate{Name: "Анна 2", Email: "anna@example.com", Password: "temp-pass-1"})
	conflict(t, err)
	_, err = e.svc.CreateStaff(ctx, service.StaffCreate{Name: "Владелец", Email: "owner@example.com", Password: "temp-pass-1"})
	conflict(t, err)

	// вход с временным паролем → смена пароля обязательна
	tok, acc, err := e.svc.Login(ctx, "anna@example.com", "temp-pass-1")
	if err != nil || !acc.MustChangePassword || acc.IsAdmin() {
		t.Fatalf("вход сотрудника: %v %+v", err, acc)
	}
	fieldErr(t, e.svc.ChangePassword(ctx, acc, tok, "temp-pass-1", "temp-pass-1"), "new_password")
	fieldErr(t, e.svc.ChangePassword(ctx, acc, tok, "temp-pass-1", "1234567"), "new_password")
	tok2, _, _ := e.svc.Login(ctx, "anna@example.com", "temp-pass-1") // второе устройство
	// сразу после входа с временным паролем текущий не спрашиваем
	if err := e.svc.ChangePassword(ctx, acc, tok, "", "anna-own-1"); err != nil {
		t.Fatal(err)
	}
	// дальше — только с верным текущим
	if err := e.svc.ChangePassword(ctx, acc, tok, "wrong-pass", "anna-own-2"); !errors.Is(err, service.ErrWrongPassword) {
		t.Fatalf("неверный текущий: %v", err)
	}
	if err := e.svc.ChangePassword(ctx, acc, tok, "anna-own-1", "anna-own-2"); err != nil {
		t.Fatal(err)
	}
	if got, err := e.svc.Session(ctx, tok); err != nil || got.MustChangePassword {
		t.Fatalf("после смены: %v %+v", err, got)
	}
	if _, err := e.svc.Session(ctx, tok2); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("другие сессии закрываются при смене пароля")
	}
	if _, _, err := e.svc.Login(ctx, "anna@example.com", "temp-pass-1"); !errors.Is(err, service.ErrBadLogin) {
		t.Fatal("временный пароль больше не подходит")
	}

	// отключить: выходит из всех сессий, войти нельзя
	off, err := e.svc.UpdateStaff(ctx, owner, st.ID, service.StaffUpdate{Name: "Анна П.", Email: "anna@example.com", Active: false})
	if err != nil || off.Active || off.Name != "Анна П." {
		t.Fatalf("отключить: %v %+v", err, off)
	}
	if _, err := e.svc.Session(ctx, tok); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("отключённый выходит из сессий")
	}
	if _, _, err := e.svc.Login(ctx, "anna@example.com", "anna-own-2"); !errors.Is(err, service.ErrBadLogin) {
		t.Fatal("отключённый не входит")
	}
	// включить и сбросить пароль
	if _, err := e.svc.UpdateStaff(ctx, owner, st.ID, service.StaffUpdate{Name: "Анна П.", Email: "anna@example.com", Active: true}); err != nil {
		t.Fatal(err)
	}
	tok, _, _ = e.svc.Login(ctx, "anna@example.com", "anna-own-2")
	rs, err := e.svc.ResetStaffPassword(ctx, owner, st.ID, "temp-pass-2")
	if err != nil || !rs.MustChangePassword {
		t.Fatalf("сброс: %v %+v", err, rs)
	}
	if _, err := e.svc.Session(ctx, tok); !errors.Is(err, service.ErrUnauthorized) {
		t.Fatal("после сброса старые сессии закрыты")
	}
	if _, acc, err := e.svc.Login(ctx, "anna@example.com", "temp-pass-2"); err != nil || !acc.MustChangePassword {
		t.Fatalf("вход с новым временным: %v", err)
	}
	fieldErr(t, func() error { _, err := e.svc.ResetStaffPassword(ctx, owner, st.ID, "x"); return err }(), "password")

	// себя не изменить и не отключить; второго администратора — можно
	list, _ := e.svc.ListStaff(ctx)
	if len(list) != 2 || list[0].Role != service.RoleAdmin {
		t.Fatalf("список: %+v", list)
	}
	_, err = e.svc.UpdateStaff(ctx, owner, owner.ID, service.StaffUpdate{Name: "x", Email: "owner@example.com", Active: false})
	conflict(t, err)
	_, err = e.svc.ResetStaffPassword(ctx, owner, owner.ID, "temp-pass-9")
	conflict(t, err)
	adm2, err := e.svc.CreateStaff(ctx, service.StaffCreate{Name: "Совладелец", Email: "co@example.com", Password: "temp-pass-1", Role: service.RoleAdmin})
	if err != nil || adm2.Role != service.RoleAdmin || !adm2.MustChangePassword {
		t.Fatalf("второй администратор: %v %+v", err, adm2)
	}
	if v, err := e.svc.UpdateStaff(ctx, owner, adm2.ID, service.StaffUpdate{Name: "Совладелец", Email: "co@example.com", Active: false}); err != nil || v.Active {
		t.Fatalf("отключить второго администратора: %v", err)
	}
	if _, err := e.svc.UpdateStaff(ctx, owner, 99999, service.StaffUpdate{Name: "x", Email: "n@example.com"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("нет такого: %v", err)
	}
	b, _ := e.svc.CreateStaff(ctx, service.StaffCreate{Name: "Борис", Email: "boris@example.com", Password: "temp-pass-1"})
	_, err = e.svc.UpdateStaff(ctx, owner, b.ID, service.StaffUpdate{Name: "Борис", Email: "anna@example.com", Active: true})
	conflict(t, err)
	fieldErr(t, func() error {
		_, err := e.svc.UpdateStaff(ctx, owner, b.ID, service.StaffUpdate{Name: "", Email: "x"})
		return err
	}(), "email")
}

func TestMenuCRUD(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreateMenuItem(ctx, service.MenuInput{Name: " ", PriceKop: 0, Category: "tea"})
	fieldErr(t, err, "name")
	fieldErr(t, err, "price_kop")
	fieldErr(t, err, "category")
	_, err = e.svc.CreateMenuItem(ctx, service.MenuInput{Name: "Раф", PriceKop: 100, Category: "milk", Description: strings.Repeat("я", 81)})
	fieldErr(t, err, "description")

	m, err := e.svc.CreateMenuItem(ctx, service.MenuInput{Name: " Матча ", PriceKop: 450_00, Category: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Матча" || !m.Visible || m.Sort != 9 {
		t.Fatalf("создано не так: %+v", m)
	}
	off := false
	m, err = e.svc.UpdateMenuItem(ctx, m.ID, service.MenuInput{Name: "Матча латте", PriceKop: 470_00, Category: "milk", Visible: &off, Signature: true, Photo: "/uploads/x.jpg"})
	if err != nil || m.Visible || !m.Signature || m.PriceKop != 470_00 {
		t.Fatalf("изменение: %v %+v", err, m)
	}
	site, _ := e.svc.Site(ctx)
	for _, x := range site.Menu {
		if x.ID == m.ID {
			t.Fatal("скрытая позиция не должна быть на сайте")
		}
	}
	if _, err := e.svc.UpdateMenuItem(ctx, 9999, service.MenuInput{Name: "x", PriceKop: 1, Category: "milk"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("нет позиции: %v", err)
	}
	if _, err := e.svc.UpdateMenuItem(ctx, m.ID, service.MenuInput{}); err == nil {
		t.Fatal("пустое изменение должно не пройти проверку")
	}
	if err := e.svc.DeleteMenuItem(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteMenuItem(ctx, m.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("повторное удаление — не найдено")
	}
}

func menuID(t *testing.T, e *env, name string) int64 {
	t.Helper()
	menu, err := e.svc.ListMenu(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range menu {
		if m.Name == name {
			return m.ID
		}
	}
	t.Fatalf("нет позиции %q", name)
	return 0
}

func TestOrderFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	latte, esp := menuID(t, e, "Латте"), menuID(t, e, "Эспрессо")

	_, err := e.svc.CreateOrder(ctx, service.OrderInput{Mode: "car", Phone: "123"})
	fieldErr(t, err, "mode")
	fieldErr(t, err, "phone")
	fieldErr(t, err, "items")
	_, err = e.svc.CreateOrder(ctx, service.OrderInput{Mode: "pickup", Phone: "+79001112233", Items: []service.OrderItemInput{{MenuItemID: latte, Qty: 0}}})
	fieldErr(t, err, "items")
	_, err = e.svc.CreateOrder(ctx, service.OrderInput{Mode: "delivery", Phone: "+79001112233", Items: []service.OrderItemInput{{MenuItemID: latte, Qty: 1}}})
	fieldErr(t, err, "address")
	_, err = e.svc.CreateOrder(ctx, service.OrderInput{Mode: "pickup", Phone: "+79001112233", Items: []service.OrderItemInput{{MenuItemID: 99999, Qty: 1}}})
	conflict(t, err)

	// Доставка — от 1000 ₽: 400 ₽ — нельзя, с подсказкой «добавьте ещё 600 ₽».
	_, err = e.svc.CreateOrder(ctx, service.OrderInput{Mode: "delivery", Phone: "+79001112233", Address: "ул. Ленина, 1", Items: []service.OrderItemInput{{MenuItemID: latte, Qty: 1}}})
	var ve *service.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Fields["mode"], "Добавьте ещё 600") {
		t.Fatalf("доставка ниже минимума: %v", err)
	}

	c, err := e.svc.CreateOrder(ctx, service.OrderInput{Mode: "delivery", Phone: "8 (900) 111-22-33", Address: "ул. Ленина, 1",
		Items: []service.OrderItemInput{{MenuItemID: latte, Qty: 2}, {MenuItemID: esp, Qty: 1}, {MenuItemID: latte, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if c.AmountKop != 3*400_00+300_00 || c.Status != "awaiting_payment" || !strings.HasPrefix(c.ConfirmationURL, "http://site.test/pay/") {
		t.Fatalf("оформление: %+v", c)
	}
	// До оплаты в кабинете заказа нет.
	if list, _ := e.svc.ListOrders(ctx); len(list) != 0 {
		t.Fatal("неоплаченный заказ не показываем")
	}
	st, err := e.svc.TestPay(ctx, c.PaymentID)
	if err != nil || st.Status != "succeeded" {
		t.Fatalf("оплата: %v %+v", err, st)
	}
	// Повторное уведомление ничего не меняет.
	if _, err := e.svc.TestPay(ctx, c.PaymentID); err != nil {
		t.Fatal(err)
	}
	p, _ := e.svc.Payment(ctx, c.PaymentID)
	if err := e.svc.HandlePaymentEvent(ctx, payments.Event{Type: payments.EventSucceeded, PaymentID: p.ID, ProviderPaymentID: "test_" + p.ID.String(), AmountKop: p.AmountKop}); err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.ListOrders(ctx)
	if len(list) != 1 || list[0].Status != "new" || list[0].Phone != "+7 (900) 111-22-33" || len(list[0].Items) != 2 {
		t.Fatalf("заказ в кабинете: %+v", list)
	}
	if len(e.email.sent) != 1 || !strings.Contains(e.email.sent[0].Body, "Латте × 3") || !strings.Contains(e.email.sent[0].Body, "Доставка: ул. Ленина, 1") {
		t.Fatalf("уведомление о заказе: %+v", e.email.sent)
	}
	if len(e.tg.sent) != 0 {
		t.Fatal("Telegram не подключён — туда не шлём")
	}
	notes, _ := e.svc.ListNotifications(ctx)
	if len(notes) != 1 || notes[0].Kind != "order" {
		t.Fatalf("журнал уведомлений: %+v", notes)
	}

	if _, err := e.svc.OrderDone(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.CancelOrder(ctx, c.ID)
	conflict(t, err)
	if _, err := e.svc.OrderDone(ctx, 9999); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("нет заказа: %v", err)
	}

	// Отмена оплаченного заказа — возврат.
	c2, _ := e.svc.CreateOrder(ctx, service.OrderInput{Mode: "pickup", Phone: "9001112233", Items: []service.OrderItemInput{{MenuItemID: esp, Qty: 1}}})
	if _, err := e.svc.TestPay(ctx, c2.PaymentID); err != nil {
		t.Fatal(err)
	}
	if o, err := e.svc.CancelOrder(ctx, c2.ID); err != nil || o.Status != "cancelled" {
		t.Fatalf("отмена: %v %+v", err, o)
	}
	if p, _ := e.svc.Payment(ctx, c2.PaymentID); p.Status != "refunded" {
		t.Fatalf("после отмены — возврат, статус %s", p.Status)
	}
}

func TestPaymentEventChecks(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, err := e.svc.CreateOrder(ctx, service.OrderInput{Mode: "pickup", Phone: "9001112233", Items: []service.OrderItemInput{{MenuItemID: menuID(t, e, "Чай"), Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	prov := "test_" + c.PaymentID.String()
	err = e.svc.HandlePaymentEvent(ctx, payments.Event{Type: payments.EventSucceeded, PaymentID: c.PaymentID, ProviderPaymentID: prov, AmountKop: 1})
	conflict(t, err) // сумма не совпала
	err = e.svc.HandlePaymentEvent(ctx, payments.Event{Type: payments.EventSucceeded, PaymentID: c.PaymentID, ProviderPaymentID: "other", AmountKop: c.AmountKop})
	conflict(t, err) // чужой платёж
	if err := e.svc.HandlePaymentEvent(ctx, payments.Event{Type: payments.EventSucceeded, PaymentID: uuid.New()}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("нет платежа: %v", err)
	}
	st, err := e.svc.TestCancel(ctx, c.PaymentID)
	if err != nil || st.Status != "canceled" {
		t.Fatalf("отказ от оплаты: %v %+v", err, st)
	}
	// после отказа оплата уже не проходит
	if st, _ := e.svc.TestPay(ctx, c.PaymentID); st.Status != "canceled" {
		t.Fatal("отменённый платёж не оплачивается")
	}
	if _, err := e.svc.Payment(ctx, uuid.New()); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("нет платежа — не найдено")
	}
	if list, _ := e.svc.ListOrders(ctx); len(list) != 0 {
		t.Fatal("неоплаченный заказ не в кабинете")
	}
}

func TestBookingFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreateBooking(ctx, service.BookingInput{})
	for _, f := range []string{"name", "phone", "date", "time", "guests"} {
		fieldErr(t, err, f)
	}
	_, err = e.svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(-1), Time: "12:00", Guests: 2})
	fieldErr(t, err, "date")
	_, err = e.svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(61), Time: "12:00", Guests: 2})
	fieldErr(t, err, "date")
	_, err = e.svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(1), Time: "21:00", Guests: 2})
	fieldErr(t, err, "time")
	_, err = e.svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(1), Time: "8:59", Guests: 11})
	fieldErr(t, err, "time")
	fieldErr(t, err, "guests")

	// Сегодня — время, которое уже прошло (часы кофейни: 21:00).
	e.svc.SetClock(func() time.Time {
		n := time.Now().In(msk)
		return time.Date(n.Year(), n.Month(), n.Day(), 20, 45, 0, 0, msk)
	})
	_, err = e.svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(0), Time: "20:30", Guests: 2})
	fieldErr(t, err, "time")
	e.svc.SetClock(time.Now)

	c, err := e.svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "+7 900 111 22 33", Date: day(1), Time: "18:00", Guests: 2, WithDog: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.AmountKop != service.BookingPrepayKop {
		t.Fatalf("предоплата 500 ₽, а не %d", c.AmountKop)
	}
	if list, _ := e.svc.ListBookings(ctx); len(list) != 0 {
		t.Fatal("неоплаченная бронь не в кабинете")
	}
	if _, err := e.svc.TestPay(ctx, c.PaymentID); err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.ListBookings(ctx)
	if len(list) != 1 || list[0].Status != "new" || list[0].Time != "18:00" || list[0].Date != day(1) || !list[0].WithDog {
		t.Fatalf("бронь в кабинете: %+v", list)
	}
	if len(e.email.sent) != 1 || !strings.Contains(e.email.sent[0].Body, "2 гостя · с собакой") || e.email.sent[0].Title != "Новая бронь столика" || e.email.sent[0].Phone == "" {
		t.Fatalf("письмо о брони: %+v", e.email.sent)
	}
	if b, err := e.svc.ConfirmBooking(ctx, c.ID); err != nil || b.Status != "confirmed" {
		t.Fatalf("подтверждение: %v %+v", err, b)
	}
	_, err = e.svc.ConfirmBooking(ctx, c.ID)
	conflict(t, err)
	if b, err := e.svc.CancelBooking(ctx, c.ID); err != nil || b.Status != "cancelled" {
		t.Fatalf("отмена: %v %+v", err, b)
	}
	if p, _ := e.svc.Payment(ctx, c.PaymentID); p.Status != "refunded" {
		t.Fatal("отмена брони — предоплата возвращается")
	}
	_, err = e.svc.CancelBooking(ctx, c.ID)
	conflict(t, err)
	if _, err := e.svc.ConfirmBooking(ctx, 9999); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("нет брони: %v", err)
	}
}

func TestEventsAndSignups(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreateEvent(ctx, service.EventInput{})
	for _, f := range []string{"title", "kind", "date", "time", "seats", "status"} {
		fieldErr(t, err, f)
	}
	_, err = e.svc.CreateEvent(ctx, service.EventInput{Title: "Прошлое", Kind: "встреча", Date: day(-1), Time: "12:00", Seats: 5, Status: "published"})
	fieldErr(t, err, "date")

	free, err := e.svc.CreateEvent(ctx, service.EventInput{Title: "Встреча", Kind: "встреча", Date: day(3), Time: "12:00", Seats: 3, Status: "published"})
	if err != nil {
		t.Fatal(err)
	}
	noDogs := false
	paid, err := e.svc.CreateEvent(ctx, service.EventInput{Title: "Латте-арт", Kind: "мастер-класс", Date: day(5), Time: "19:00", Seats: 4, PriceKop: 700_00, DogsAllowed: &noDogs, Status: "published"})
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := e.svc.CreateEvent(ctx, service.EventInput{Title: "Черновик", Kind: "другое", Date: day(6), Time: "18:00", Seats: 4, Status: "draft"})

	site, _ := e.svc.Site(ctx)
	if len(site.Events) != 2 || site.Events[0].ID != free.ID {
		t.Fatalf("на сайте — только опубликованные, по дате: %+v", site.Events)
	}

	// бесплатная запись — сразу подтверждена
	_, err = e.svc.CreateSignup(ctx, free.ID, service.SignupInput{})
	fieldErr(t, err, "name")
	fieldErr(t, err, "via")
	fieldErr(t, err, "people")
	_, err = e.svc.CreateSignup(ctx, free.ID, service.SignupInput{Name: "Мария", Via: "tg", Contact: "@a", People: 1})
	fieldErr(t, err, "contact")
	_, err = e.svc.CreateSignup(ctx, free.ID, service.SignupInput{Name: "Мария", Via: "phone", Contact: "12", People: 1})
	fieldErr(t, err, "contact")
	c, err := e.svc.CreateSignup(ctx, free.ID, service.SignupInput{Name: "Мария", Via: "tg", Contact: "maria_corgi", People: 2, WithDog: true})
	if err != nil || c.Status != "confirmed" || c.ConfirmationURL != "" {
		t.Fatalf("бесплатная запись: %v %+v", err, c)
	}
	_, err = e.svc.CreateSignup(ctx, free.ID, service.SignupInput{Name: "Игорь", Via: "phone", Contact: "9001112233", People: 2})
	conflict(t, err) // осталось 1 место
	if _, err := e.svc.CreateSignup(ctx, free.ID, service.SignupInput{Name: "Игорь", Via: "phone", Contact: "9001112233", People: 1}); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.CreateSignup(ctx, free.ID, service.SignupInput{Name: "Ещё", Via: "phone", Contact: "9001112233", People: 1})
	conflict(t, err) // мест нет
	kinds := map[string]int{}
	for _, m := range e.email.sent {
		kinds[m.Kind]++
	}
	if kinds["signup"] != 2 || kinds["full"] != 1 {
		t.Fatalf("уведомления о записях и «мест нет»: %v", kinds)
	}
	su, _ := e.svc.ListSignups(ctx, free.ID)
	if len(su) != 2 || su[0].Contact != "@maria_corgi" || su[1].Contact != "+7 (900) 111-22-33" {
		t.Fatalf("записавшиеся: %+v", su)
	}

	// платная — место держится на время оплаты, в кабинете после оплаты
	c, err = e.svc.CreateSignup(ctx, paid.ID, service.SignupInput{Name: "Оля", Via: "phone", Contact: "9001112233", People: 3, WithDog: true})
	if err != nil || c.AmountKop != 3*700_00 || c.Status != "awaiting_payment" {
		t.Fatalf("платная запись: %v %+v", err, c)
	}
	_, err = e.svc.CreateSignup(ctx, paid.ID, service.SignupInput{Name: "Паша", Via: "phone", Contact: "9001112233", People: 2})
	conflict(t, err) // 3 места держатся
	if su, _ := e.svc.ListSignups(ctx, paid.ID); len(su) != 0 {
		t.Fatal("до оплаты запись не показываем")
	}
	if _, err := e.svc.TestPay(ctx, c.PaymentID); err != nil {
		t.Fatal(err)
	}
	su, _ = e.svc.ListSignups(ctx, paid.ID)
	if len(su) != 1 || su[0].People != 3 || su[0].WithDog {
		t.Fatalf("после оплаты (с собакой нельзя — флаг снят): %+v", su)
	}
	evs, _ := e.svc.ListEvents(ctx)
	for _, x := range evs {
		if x.ID == paid.ID && (x.Taken != 3 || x.SeatsLeft != 1) {
			t.Fatalf("места: %+v", x)
		}
	}

	// мест не меньше, чем записалось
	_, err = e.svc.UpdateEvent(ctx, paid.ID, service.EventInput{Title: "Латте-арт", Kind: "мастер-класс", Date: day(5), Time: "19:00", Seats: 2, PriceKop: 700_00, Status: "published"})
	fieldErr(t, err, "seats")
	off := false
	upd, err := e.svc.UpdateEvent(ctx, paid.ID, service.EventInput{Title: "Латте-арт 2", Kind: "мастер-класс", Date: day(5), Time: "19:30", Seats: 6, PriceKop: 700_00, SignupEnabled: &off, Status: "published"})
	if err != nil || upd.Time != "19:30" || upd.SeatsLeft != 3 || upd.SignupEnabled {
		t.Fatalf("изменение: %v %+v", err, upd)
	}
	_, err = e.svc.CreateSignup(ctx, paid.ID, service.SignupInput{Name: "Паша", Via: "phone", Contact: "9001112233", People: 1})
	conflict(t, err) // запись закрыта
	if _, err := e.svc.CreateSignup(ctx, draft.ID, service.SignupInput{Name: "Паша", Via: "phone", Contact: "9001112233", People: 1}); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("на черновик записаться нельзя")
	}
	if _, err := e.svc.UpdateEvent(ctx, 9999, service.EventInput{Title: "x", Kind: "встреча", Date: day(1), Time: "12:00", Seats: 1, Status: "draft"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("нет мероприятия")
	}
	if _, err := e.svc.ListSignups(ctx, 9999); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("нет мероприятия — записи")
	}
	if err := e.svc.DeleteEvent(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteEvent(ctx, draft.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatal("повторное удаление")
	}
}

func TestContentAndSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, err := e.svc.Content(ctx)
	if err != nil || c.Address != "ул. Сарьяна, 34" || len(c.Why) != 3 || len(c.Dogs) != 4 {
		t.Fatalf("тексты по умолчанию: %v %+v", err, c)
	}
	bad := c
	bad.HeroChip, bad.Why = "", c.Why[:2]
	_, err = e.svc.PutContent(ctx, bad)
	fieldErr(t, err, "hero_chip")
	fieldErr(t, err, "why")

	c.HowToGet = "Вход со двора"
	c.Dogs = append(c.Dogs, service.Dog{}, service.Dog{Name: "Бублик", About: "шпиц", Photo: "/uploads/b.jpg"})
	saved, err := e.svc.PutContent(ctx, c)
	if err != nil || len(saved.Dogs) != 5 || saved.Dogs[4].Name != "Бублик" {
		t.Fatalf("сохранение (пустая собака отброшена): %v %+v", err, saved.Dogs)
	}
	site, _ := e.svc.Site(ctx)
	if site.Content.HowToGet != "Вход со двора" || site.Order.DeliveryMinKop != 1000_00 || site.Order.DeliveryRadiusKm != 10 || site.Order.BookingPrepayKop != 500_00 {
		t.Fatalf("сайт: %+v", site.Order)
	}

	s, _ := e.svc.Settings(ctx)
	s.DeliveryMinKop, s.DeliveryRadiusKm = -1, 0
	s.Notify.Email = "не почта"
	_, err = e.svc.PutSettings(ctx, s)
	fieldErr(t, err, "delivery_min_kop")
	fieldErr(t, err, "delivery_radius_km")
	fieldErr(t, err, "email")

	s.DeliveryMinKop, s.DeliveryRadiusKm = 1500_00, 8
	s.Notify.Email = "owner@example.com"
	s.Notify.TelegramConnected = true // меняется только через connect
	s.Notify.OnOrder = false
	s, err = e.svc.PutSettings(ctx, s)
	if err != nil || s.DeliveryMinKop != 1500_00 || s.Notify.TelegramConnected || s.Notify.OnOrder {
		t.Fatalf("настройки: %v %+v", err, s)
	}

	_, err = e.svc.TestNotify(ctx, "telegram")
	conflict(t, err)
	_, err = e.svc.TestNotify(ctx, "pigeon")
	fieldErr(t, err, "channel")
	if n, err := e.svc.TestNotify(ctx, "email"); err != nil || n.Recipient != "owner@example.com" || n.Status != "sent" {
		t.Fatalf("проверочное письмо: %v %+v", err, n)
	}
	r, err := e.svc.TelegramConnect(ctx)
	s = r.Settings
	if err != nil || r.Link != "" || !s.Notify.TelegramConnected || s.Notify.TelegramChat != "@ludi_bydte_dobree" || len(e.tg.sent) != 1 {
		t.Fatalf("подключение Telegram: %v %+v", err, s.Notify)
	}
	e.tg.fail = true
	if n, _ := e.svc.TestNotify(ctx, "telegram"); n.Status != "failed" {
		t.Fatal("сбой канала записывается в журнал как failed")
	}
	s, _ = e.svc.TelegramDisconnect(ctx)
	if s.Notify.TelegramConnected {
		t.Fatal("отключение Telegram")
	}

	// Письма выключены и почты нет — проверка требует почту.
	s.Notify.EmailEnabled, s.Notify.Email = false, ""
	if _, err := e.svc.PutSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.TestNotify(ctx, "email")
	fieldErr(t, err, "email")
	s.Notify.EmailEnabled = true
	_, err = e.svc.PutSettings(ctx, s)
	fieldErr(t, err, "email")
}
