package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kofesobaken/backend/internal/notify"
)

func TestResendSend(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"id":"e1"}`)
	}))
	defer srv.Close()
	ch := notify.Resend{APIKey: "re_test", BaseURL: srv.URL}
	if ch.Name() != "email" || notify.IsLog(ch) {
		t.Fatal("канал почты")
	}
	err := ch.Send(context.Background(), "owner@example.com", notify.Message{
		Kind: "booking", Title: "Новая бронь столика", Body: "Анна · +7 900 000-00-01\nСб, 17 октября, 18:00 <b>", Phone: "+7 900 000-00-01", Link: "https://site.test/admin#bookings",
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_test" || path != "/emails" {
		t.Fatalf("запрос: %q %q", auth, path)
	}
	html, _ := got["html"].(string)
	if got["from"] != notify.DefaultMailFrom || got["subject"] != "Новая бронь столика" || got["to"].([]any)[0] != "owner@example.com" {
		t.Fatalf("письмо: %+v", got)
	}
	for _, want := range []string{"Анна", "18:00 &lt;b&gt;", `href="tel:+79000000001"`, `href="https://site.test/admin#bookings"`, "Открыть кабинет"} {
		if !strings.Contains(html, want) {
			t.Fatalf("в письме нет %q:\n%s", want, html)
		}
	}
	if !strings.Contains(got["text"].(string), "https://site.test/admin#bookings") {
		t.Fatal("текстовая версия письма")
	}
	if _, ok := got["reply_to"]; ok {
		t.Fatal("reply_to без адреса")
	}
	// вопрос гостя: «Ответить» в почте — сразу гостю
	_ = ch.Send(context.Background(), "owner@example.com", notify.Message{Kind: "question", Title: "Вопрос", Body: "?", ReplyTo: "guest@example.com"})
	if got["reply_to"] != "guest@example.com" {
		t.Fatalf("reply_to: %+v", got["reply_to"])
	}
}

func TestResendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"You can only send testing emails to your own email address"}`)
	}))
	defer srv.Close()
	err := notify.Resend{APIKey: "k", BaseURL: srv.URL}.Send(context.Background(), "x@example.com", notify.Message{Title: "t"})
	if err == nil || !strings.Contains(err.Error(), "own email") {
		t.Fatalf("ошибка Resend: %v", err)
	}
	if err := (notify.Resend{APIKey: "k", BaseURL: srv.URL}).Send(context.Background(), "", notify.Message{}); err == nil {
		t.Fatal("без адреса")
	}
}

// tgStub — заглушка Bot API: запоминает вызовы, отдаёт заданные updates.
type tgStub struct {
	calls   map[string][]map[string]any
	updates string
	fail    bool
}

func (s *tgStub) server(t *testing.T) *httptest.Server {
	s.calls = map[string][]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if !strings.HasPrefix(r.URL.Path, "/botTOKEN/") {
			t.Errorf("путь: %s", r.URL.Path)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		s.calls[method] = append(s.calls[method], in)
		if s.fail {
			_, _ = io.WriteString(w, `{"ok":false,"description":"Forbidden: bot was blocked by the user"}`)
			return
		}
		switch method {
		case "getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"username":"kofesobaken_bot"}}`)
		case "getUpdates":
			_, _ = io.WriteString(w, `{"ok":true,"result":`+s.updates+`}`)
		default:
			_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTelegramSend(t *testing.T) {
	st := &tgStub{}
	srv := st.server(t)
	tg := notify.Telegram{Token: "TOKEN", BaseURL: srv.URL, PublicURL: "https://site.test/"}
	err := tg.Send(context.Background(), "42", notify.Message{Title: "Новый заказ №7", Body: "Латте × 2\n+7 900 000-00-01", Phone: "+7 900 000-00-01", Link: "https://site.test/admin#orders"})
	if err != nil {
		t.Fatal(err)
	}
	in := st.calls["sendMessage"][0]
	if in["chat_id"] != "42" || !strings.HasPrefix(in["text"].(string), "Новый заказ №7\nЛатте") {
		t.Fatalf("сообщение: %+v", in)
	}
	raw, _ := json.Marshal(in["reply_markup"])
	for _, want := range []string{`"Позвонить"`, `"https://site.test/api/call/+79000000001"`, `"Открыть кабинет"`, `"https://site.test/admin#orders"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("кнопки: нет %s в %s", want, raw)
		}
	}
	// адрес сайта не https (локально) — без кнопок: Telegram такие ссылки не принимает
	tg.PublicURL = "http://localhost:8000"
	_ = tg.Send(context.Background(), "42", notify.Message{Title: "t", Phone: "+79000000001"})
	if _, ok := st.calls["sendMessage"][1]["reply_markup"]; ok {
		t.Fatal("кнопки на http-адресе")
	}
	if err := tg.Send(context.Background(), "", notify.Message{}); err == nil {
		t.Fatal("без чата")
	}
	st.fail = true
	if err := tg.Send(context.Background(), "42", notify.Message{}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("ошибка Bot API: %v", err)
	}
}

func TestTelegramUpdates(t *testing.T) {
	st := &tgStub{updates: `[
		{"update_id":10,"message":{"message_id":3,"text":"привет","chat":{"id":5,"type":"private","first_name":"Вова","last_name":"О."}}},
		{"update_id":11,"message":{"message_id":4,"text":"/start abc","chat":{"id":7,"type":"private","username":"owner","first_name":"Вова"}}},
		{"update_id":12,"message":{"text":"/start abc","chat":{"id":-100,"type":"group"}}},
		{"update_id":13},
		{"update_id":14,"message":{"message_id":9,"caption":"а это можно?","chat":{"id":7,"type":"private","username":"owner"},"reply_to_message":{"message_id":77}}}
	]`}
	srv := st.server(t)
	tg := notify.Telegram{Token: "TOKEN", BaseURL: srv.URL}
	if u, err := tg.BotUsername(context.Background()); err != nil || u != "kofesobaken_bot" {
		t.Fatalf("бот: %q %v", u, err)
	}
	ups, next, err := tg.Updates(context.Background(), 9, 0)
	if err != nil || next != 15 || len(ups) != 3 {
		t.Fatalf("updates: %+v %d %v", ups, next, err)
	}
	if u := ups[0]; u.ChatID != 5 || u.MessageID != 3 || u.Who() != "Вова О." || u.Text != "привет" {
		t.Fatalf("сообщение гостя: %+v", u)
	}
	if _, ok := ups[0].StartCode(); ok {
		t.Fatal("обычный текст — не /start")
	}
	if c, ok := ups[1].StartCode(); !ok || c != "abc" || ups[1].Who() != "@owner" {
		t.Fatalf("старт: %+v", ups[1])
	}
	if u := ups[2]; u.Text != "а это можно?" || u.ReplyTo != 77 {
		t.Fatalf("реплай с подписью: %+v", u)
	}
	if st.calls["getUpdates"][0]["offset"] != float64(9) {
		t.Fatal("offset")
	}
	if _, err := tg.Reply(context.Background(), 5, "ответ", 3); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(st.calls["sendMessage"][0]["reply_parameters"])
	if !strings.Contains(string(raw), `"message_id":3`) {
		t.Fatalf("реплай гостю: %s", raw)
	}
}
