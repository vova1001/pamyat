package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/service"
)

// botStub — заглушка Telegram Bot API для подключения чата.
type botStub struct {
	mu      sync.Mutex
	updates []string
	sent    []map[string]any
	offsets []float64
}

func (b *botStub) start(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = io.WriteString(w, `{"ok":true,"result":{"username":"kofesobaken_bot"}}`)
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			off, _ := in["offset"].(float64)
			b.offsets = append(b.offsets, off)
			_, _ = io.WriteString(w, `{"ok":true,"result":[`+strings.Join(b.updates, ",")+`]}`)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			b.sent = append(b.sent, in)
			_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTelegramConnectBot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	bot := &botStub{}
	srv := bot.start(t)
	svc := service.New(testPool, e.pay, []notify.Channel{e.email, notify.Telegram{Token: "T", BaseURL: srv.URL, PublicURL: "https://site.test"}}, msk)
	svc.SetPublicURL("https://site.test")

	// подключено заглушкой (до бота) — с ботом считается неподключённым: chat_id нет
	if _, err := e.svc.TelegramConnect(ctx); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Settings(ctx); s.Notify.TelegramConnected {
		t.Fatal("подключение заглушкой при настоящем боте")
	}
	// ещё не нажимали «Подключить» — проверка ничего не делает
	if s, err := svc.TelegramCheck(ctx); err != nil || s.Notify.TelegramConnected || len(bot.offsets) != 0 {
		t.Fatalf("проверка без ссылки: %v %+v", err, s.Notify)
	}
	r, err := svc.TelegramConnect(ctx)
	if err != nil || !strings.HasPrefix(r.Link, "https://t.me/kofesobaken_bot?start=") || r.Notify.TelegramConnected {
		t.Fatalf("ссылка подключения: %v %+v", err, r)
	}
	code := strings.TrimPrefix(r.Link, "https://t.me/kofesobaken_bot?start=")

	// чужой код и сообщения без кода — не подключают; offset сдвигается
	bot.updates = []string{`{"update_id":100,"message":{"text":"/start чужой","chat":{"id":1,"type":"private","first_name":"X"}}}`}
	if s, _ := svc.TelegramCheck(ctx); s.Notify.TelegramConnected {
		t.Fatal("чужой код подключил чат")
	}
	bot.updates = []string{fmt.Sprintf(`{"update_id":101,"message":{"text":"/start %s","chat":{"id":555,"type":"private","username":"owner","first_name":"Вова"}}}`, code)}
	s, err := svc.TelegramCheck(ctx)
	if err != nil || !s.Notify.TelegramConnected || s.Notify.TelegramChat != "@owner" || s.Notify.TelegramChatID != "555" {
		t.Fatalf("подключение: %v %+v", err, s.Notify)
	}
	if bot.offsets[1] != 101 {
		t.Fatalf("offset getUpdates: %v", bot.offsets)
	}
	if len(bot.sent) != 1 || bot.sent[0]["chat_id"] != "555" {
		t.Fatalf("проверочное сообщение: %+v", bot.sent)
	}
	// код одноразовый: повторная проверка ничего не трогает
	if _, err := svc.TelegramCheck(ctx); err != nil || len(bot.offsets) != 2 {
		t.Fatalf("повторная проверка: %v %v", err, bot.offsets)
	}
	// настройки из кабинета не сбрасывают chat_id
	s.Notify.TelegramChatID = "999"
	if s, err = svc.PutSettings(ctx, s); err != nil || s.Notify.TelegramChatID != "555" {
		t.Fatalf("chat_id после сохранения настроек: %v %+v", err, s.Notify)
	}
	// уведомление о заказе уходит в чат 555 с кнопкой «Позвонить»
	if n, err := svc.TestNotify(ctx, "telegram"); err != nil || n.Recipient != "555" || n.Status != "sent" {
		t.Fatalf("проверка: %v %+v", err, n)
	}
	s, _ = svc.TelegramDisconnect(ctx)
	if s.Notify.TelegramConnected || s.Notify.TelegramChatID != "" {
		t.Fatal("отключение")
	}

	// ссылка устарела — «Старт» с ней не подключает
	r, _ = svc.TelegramConnect(ctx)
	code = strings.TrimPrefix(r.Link, "https://t.me/kofesobaken_bot?start=")
	bot.updates = []string{fmt.Sprintf(`{"update_id":102,"message":{"text":"/start %s","chat":{"id":555,"type":"private","first_name":"Вова"}}}`, code)}
	svc.SetClock(func() time.Time { return time.Now().Add(service.TelegramLinkTTL + time.Minute) })
	if s, _ := svc.TelegramCheck(ctx); s.Notify.TelegramConnected {
		t.Fatal("устаревшая ссылка подключила чат")
	}
}

func TestForgotPassword(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// без адреса сайта ссылку не собрать — письмо не уходит
	if err := e.svc.ForgotPassword(ctx, "owner@example.com"); err != nil || len(e.email.sent) != 0 {
		t.Fatalf("без адреса сайта: %v", err)
	}
	e.svc.SetPublicURL("https://site.test/")
	if !e.svc.MailConfigured() {
		t.Fatal("почтовый канал")
	}
	if err := e.svc.ForgotPassword(ctx, "nobody@example.com"); err != nil || len(e.email.sent) != 0 {
		t.Fatalf("неизвестная почта: %v", err)
	}
	if err := e.svc.ForgotPassword(ctx, " Owner@Example.com "); err != nil || len(e.email.sent) != 1 {
		t.Fatalf("письмо: %v %d", err, len(e.email.sent))
	}
	m := e.email.sent[0]
	code, ok := strings.CutPrefix(m.Link, "https://site.test/admin#setup/")
	if m.Kind != "reset" || !ok || code == "" {
		t.Fatalf("ссылка в письме: %+v", m)
	}
	tok, acc, err := e.svc.SetupPassword(ctx, code, "new-owner-pass")
	if err != nil || acc.Email != "owner@example.com" {
		t.Fatalf("новый пароль: %v", err)
	}
	if _, err := e.svc.Session(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, "owner@example.com", "new-owner-pass"); err != nil {
		t.Fatal("вход новым паролем")
	}

	// сотрудник; ссылка живёт 1 час; отключённому — ни письма, ни входа по старой ссылке
	owner := acc
	st, err := e.svc.CreateStaff(ctx, service.StaffCreate{Name: "Бариста", Email: "barista@example.com", Password: "temp-pass-1"})
	if err != nil {
		t.Fatal(err)
	}
	_ = e.svc.ForgotPassword(ctx, "barista@example.com")
	code = strings.TrimPrefix(e.email.sent[1].Link, "https://site.test/admin#setup/")
	if em, err := e.svc.SetupEmail(ctx, code); err != nil || em != "barista@example.com" {
		t.Fatalf("ссылка сотрудника: %q %v", em, err)
	}
	if _, err := e.svc.UpdateStaff(ctx, owner, st.ID, service.StaffUpdate{Name: "Бариста", Email: "barista@example.com", Active: false}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.SetupPassword(ctx, code, "barista-pass-2"); err == nil {
		t.Fatal("отключённый сотрудник вошёл по ссылке")
	}
	_ = e.svc.ForgotPassword(ctx, "barista@example.com")
	if len(e.email.sent) != 2 {
		t.Fatal("письмо отключённому сотруднику")
	}

	// срок ссылки — 1 час
	e.svc.SetClock(func() time.Time { return time.Now().Add(-service.ResetTTL - time.Minute) })
	_ = e.svc.ForgotPassword(ctx, "owner@example.com")
	e.svc.SetClock(time.Now)
	code = strings.TrimPrefix(e.email.sent[2].Link, "https://site.test/admin#setup/")
	if _, err := e.svc.SetupEmail(ctx, code); err == nil {
		t.Fatal("ссылка старше часа действует")
	}

	// без почтового ключа — ничего
	plain := service.New(testPool, e.pay, []notify.Channel{notify.LogChannel{Channel_: "email"}}, msk)
	plain.SetPublicURL("https://site.test")
	if plain.MailConfigured() {
		t.Fatal("заглушка — не почта")
	}
	if err := plain.ForgotPassword(ctx, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
}
