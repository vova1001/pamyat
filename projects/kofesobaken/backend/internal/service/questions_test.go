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

func TestQuestionFromSite(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.svc.SetPublicURL("https://site.test")

	// проверка полей
	_, err := e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "", Via: "email", Contact: "anna@example.com", Question: "Можно с котом?"})
	fieldErr(t, err, "name")
	_, err = e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "Анна", Via: "email", Contact: "anna@", Question: "Можно с котом?"})
	fieldErr(t, err, "contact")
	_, err = e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "Анна", Via: "telegram", Contact: "@ан", Question: "Можно с котом?"})
	fieldErr(t, err, "contact")
	_, err = e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "Анна", Via: "email", Contact: "anna@example.com", Question: "?"})
	fieldErr(t, err, "question")
	_, err = e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "Анна", Via: "pigeon", Contact: "x", Question: "Можно с котом?"})
	fieldErr(t, err, "via")
	// ловушка для ботов: «принято», но не сохранено и не отправлено
	if _, err := e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "bot", Via: "email", Contact: "b@b.bb", Question: "spam spam", Website: "x"}); err != nil {
		t.Fatal(err)
	}

	q, err := e.svc.CreateQuestion(ctx, service.QuestionInput{Name: " Анна ", Via: "email", Contact: "Anna@Example.com", Question: "Можно прийти с котом?"})
	if err != nil {
		t.Fatal(err)
	}
	if q.Contact != "anna@example.com" || q.Source != "site" || q.Name != "Анна" || q.CanSend {
		t.Fatalf("вопрос: %+v", q)
	}
	// владелице — письмо с Reply-To гостя и сообщение в Telegram
	if len(e.email.sent) != 1 || e.email.sent[0].ReplyTo != "anna@example.com" || e.email.sent[0].Kind != "question" ||
		!strings.Contains(e.email.sent[0].Body, "Можно прийти с котом?") || e.email.sent[0].Link != "https://site.test/admin#questions" {
		t.Fatalf("письмо владелице: %+v", e.email.sent)
	}
	if len(e.tg.sent) != 0 { // Telegram ещё не подключён
		t.Fatalf("telegram: %+v", e.tg.sent)
	}

	// без подтверждённого домена — ответ из кабинета письмом не уходит, только отметка «отвечено»
	_, err = e.svc.AnswerQuestion(ctx, q.ID, "Да, конечно!", true)
	conflict(t, err)
	a, err := e.svc.AnswerQuestion(ctx, q.ID, "Да, конечно!", false)
	if err != nil || a.AnsweredAt == nil || a.AnsweredVia != "manual" || a.Answer != "Да, конечно!" {
		t.Fatalf("отметка «отвечено»: %v %+v", err, a)
	}
	// домен подтверждён — письмо гостю, «Ответить» — владелице
	e.svc.SetMailDomainVerified(true)
	a, err = e.svc.AnswerQuestion(ctx, q.ID, "Да, ждём вас с котом", true)
	if err != nil || a.AnsweredVia != "email" {
		t.Fatalf("ответ письмом: %v %+v", err, a)
	}
	m := e.email.sent[len(e.email.sent)-1]
	if m.Kind != "answer" || m.ReplyTo != "vovaoleshko05@mail.ru" || !strings.Contains(m.Body, "Да, ждём вас с котом") || !strings.Contains(m.Body, "Можно прийти с котом?") {
		t.Fatalf("письмо гостю: %+v", m)
	}
	_, err = e.svc.AnswerQuestion(ctx, q.ID, "", true)
	fieldErr(t, err, "answer")
	_, err = e.svc.AnswerQuestion(ctx, 999999, "x", false)
	if err != service.ErrNotFound {
		t.Fatalf("нет вопроса: %v", err)
	}

	// ник в Telegram: ответ — владелица пишет сама
	q2, err := e.svc.CreateQuestion(ctx, service.QuestionInput{Name: "Олег", Via: "telegram", Contact: "oleg_k", Question: "Есть розетки?"})
	if err != nil || q2.Contact != "@oleg_k" || q2.CanSend {
		t.Fatalf("вопрос с ником: %v %+v", err, q2)
	}
	_, err = e.svc.AnswerQuestion(ctx, q2.ID, "Есть", true)
	conflict(t, err)

	list, err := e.svc.ListQuestions(ctx)
	if err != nil || len(list.Items) != 2 || list.Items[0].ID != q2.ID || !list.MailDirect {
		t.Fatalf("список: %v %+v", err, list)
	}

	// без почтового канала (нет ключа) — сохраняется в кабинет, ничего не падает
	plain := service.New(testPool, e.pay, []notify.Channel{notify.LogChannel{Channel_: "email"}, notify.LogChannel{Channel_: "telegram"}}, msk)
	plain.SetMailDomainVerified(true)
	if _, err := plain.CreateQuestion(ctx, service.QuestionInput{Name: "Ира", Via: "email", Contact: "ira@example.com", Question: "Во сколько открываетесь?"}); err != nil {
		t.Fatal(err)
	}
	if l, _ := plain.ListQuestions(ctx); l.MailDirect || l.Items[0].CanSend {
		t.Fatal("без ключа почты ответ «уходит»")
	}
}

// questionBot — заглушка Bot API: входящие по очереди, отправленные с message_id.
type questionBot struct {
	mu      sync.Mutex
	updates []string
	sent    []map[string]any
	nextID  int64
	failTo  float64 // chat_id, которому sendMessage отвечает ошибкой
}

func (b *questionBot) start(t *testing.T) *httptest.Server {
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
			var out []string
			for _, u := range b.updates {
				var x struct {
					ID float64 `json:"update_id"`
				}
				_ = json.Unmarshal([]byte(u), &x)
				if x.ID >= off {
					out = append(out, u)
				}
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":[`+strings.Join(out, ",")+`]}`)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			if c, _ := in["chat_id"].(float64); c != 0 && c == b.failTo {
				_, _ = io.WriteString(w, `{"ok":false,"description":"Forbidden: bot was blocked by the user"}`)
				return
			}
			b.sent = append(b.sent, in)
			b.nextID++
			fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, 1000+b.nextID)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (b *questionBot) push(u string) { b.mu.Lock(); b.updates = append(b.updates, u); b.mu.Unlock() }

func (b *questionBot) last() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sent[len(b.sent)-1]
}

func TestQuestionsViaTelegramBot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	bot := &questionBot{}
	srv := bot.start(t)
	svc := service.New(testPool, e.pay, []notify.Channel{e.email, notify.Telegram{Token: "T", BaseURL: srv.URL, PublicURL: "https://site.test"}}, msk)
	svc.SetPublicURL("https://site.test")

	// подключение владелицы — тем же обработчиком
	r, err := svc.TelegramConnect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if svc.BotName() != "kofesobaken_bot" {
		t.Fatalf("имя бота: %q", svc.BotName())
	}
	code := strings.TrimPrefix(r.Link, "https://t.me/kofesobaken_bot?start=")
	bot.push(fmt.Sprintf(`{"update_id":1,"message":{"message_id":1,"text":"/start %s","chat":{"id":555,"type":"private","username":"owner"}}}`, code))
	if err := svc.PollTelegram(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Settings(ctx); s.Notify.TelegramChatID != "555" {
		t.Fatalf("владелица не подключена: %+v", s.Notify)
	}
	sentBefore := len(bot.sent) // проверочное сообщение

	// гость: /start → приветствие; вопрос → владелице с подсказкой про реплай, гостю «спасибо»
	bot.push(`{"update_id":2,"message":{"message_id":10,"text":"/start","chat":{"id":777,"type":"private","username":"guest","first_name":"Маша"}}}`)
	bot.push(`{"update_id":3,"message":{"message_id":11,"text":"Можно у вас отметить день рождения собаки?","chat":{"id":777,"type":"private","username":"guest","first_name":"Маша"}}}`)
	if err := svc.PollTelegram(ctx, 0); err != nil {
		t.Fatal(err)
	}
	sent := bot.sent[sentBefore:]
	if len(sent) != 3 {
		t.Fatalf("сообщения бота: %+v", sent)
	}
	if sent[0]["chat_id"] != float64(777) || !strings.Contains(sent[0]["text"].(string), "Напишите свой вопрос") {
		t.Fatalf("приветствие: %+v", sent[0])
	}
	if sent[1]["chat_id"] != "555" || !strings.Contains(sent[1]["text"].(string), "день рождения собаки") || !strings.Contains(sent[1]["text"].(string), "реплаем") {
		t.Fatalf("вопрос владелице: %+v", sent[1])
	}
	ownerMsg := 1000 + int64(sentBefore) + 2 // id сообщения с вопросом в чате владелицы
	if sent[2]["chat_id"] != float64(777) || !strings.Contains(sent[2]["text"].(string), "Спасибо") {
		t.Fatalf("гостю «спасибо»: %+v", sent[2])
	}
	list, _ := svc.ListQuestions(ctx)
	if len(list.Items) != 1 || list.Items[0].Source != "telegram" || list.Items[0].Contact != "@guest" || list.Items[0].Name != "Маша" || !list.Items[0].CanSend {
		t.Fatalf("вопрос в кабинете: %+v", list.Items)
	}
	qid := list.Items[0].ID
	// письмо владелице тоже ушло (почта включена по умолчанию)
	if len(e.email.sent) != 1 || e.email.sent[0].Kind != "question" || e.email.sent[0].ReplyTo != "" {
		t.Fatalf("письмо: %+v", e.email.sent)
	}

	// повторная выдача тех же сообщений (offset не сохранился) — вопрос не дублируется
	if err := svc.PollTelegram(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.ListQuestions(ctx); len(l.Items) != 1 {
		t.Fatal("дубль вопроса")
	}

	// второе сообщение гостя сразу следом — без повторного «спасибо»
	n := len(bot.sent)
	bot.push(`{"update_id":4,"message":{"message_id":12,"text":"Нас будет 6 человек","chat":{"id":777,"type":"private","username":"guest","first_name":"Маша"}}}`)
	_ = svc.PollTelegram(ctx, 0)
	if len(bot.sent) != n+1 || bot.last()["chat_id"] != "555" {
		t.Fatalf("второй вопрос: %+v", bot.sent[n:])
	}

	// владелица пишет не реплаем — подсказка
	bot.push(`{"update_id":5,"message":{"message_id":2,"text":"привет","chat":{"id":555,"type":"private","username":"owner"}}}`)
	_ = svc.PollTelegram(ctx, 0)
	if bot.last()["chat_id"] != float64(555) || !strings.Contains(bot.last()["text"].(string), "реплаем") {
		t.Fatalf("подсказка владелице: %+v", bot.last())
	}

	// реплай владелицы — ответ гостю реплаем на его сообщение, отметка в кабинете, подтверждение владелице
	bot.push(fmt.Sprintf(`{"update_id":6,"message":{"message_id":3,"text":"Да! Приходите, украсим зал","chat":{"id":555,"type":"private","username":"owner"},"reply_to_message":{"message_id":%d}}}`, ownerMsg))
	n = len(bot.sent)
	if err := svc.PollTelegram(ctx, 0); err != nil {
		t.Fatal(err)
	}
	got := bot.sent[n:]
	if len(got) != 2 || got[0]["chat_id"] != float64(777) || !strings.Contains(got[0]["text"].(string), "Да! Приходите, украсим зал") {
		t.Fatalf("ответ гостю: %+v", got)
	}
	if rp, _ := got[0]["reply_parameters"].(map[string]any); rp["message_id"] != float64(11) {
		t.Fatalf("ответ — реплаем на вопрос гостя: %+v", got[0])
	}
	if got[1]["chat_id"] != float64(555) || !strings.Contains(got[1]["text"].(string), "Отправлено") {
		t.Fatalf("подтверждение владелице: %+v", got[1])
	}
	list, _ = svc.ListQuestions(ctx)
	var q service.Question
	for _, x := range list.Items {
		if x.ID == qid {
			q = x
		}
	}
	if q.AnsweredAt == nil || q.AnsweredVia != "telegram" || q.Answer != "Да! Приходите, украсим зал" {
		t.Fatalf("отметка «отвечено»: %+v", q)
	}

	// ответ из кабинета — тем же ботом
	a, err := svc.AnswerQuestion(ctx, list.Items[0].ID, "Ждём 6 человек", true)
	if err != nil || a.AnsweredVia != "telegram" || bot.last()["chat_id"] != float64(777) {
		t.Fatalf("ответ из кабинета: %v %+v", err, a)
	}
	// гость заблокировал бота — понятная ошибка, ответ не отмечен
	bot.failTo = 777
	_, err = svc.AnswerQuestion(ctx, list.Items[0].ID, "Ещё раз", true)
	conflict(t, err)

	// реплай владелицы на вопрос с сайта (почта, домена нет) — бот подсказывает ответить письмом
	sq, err := svc.CreateQuestion(ctx, service.QuestionInput{Name: "Анна", Via: "email", Contact: "anna@example.com", Question: "Есть веганское молоко?"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bot.last()["text"].(string), "письмом на anna@example.com") {
		t.Fatalf("вопрос с сайта в Telegram: %+v", bot.last())
	}
	siteMsg := 1000 + bot.nextID
	bot.push(fmt.Sprintf(`{"update_id":7,"message":{"message_id":4,"text":"Есть овсяное","chat":{"id":555,"type":"private"},"reply_to_message":{"message_id":%d}}}`, siteMsg))
	_ = svc.PollTelegram(ctx, 0)
	if !strings.Contains(bot.last()["text"].(string), "Отсюда гостю не отправить") {
		t.Fatalf("реплай на вопрос с почтой: %+v", bot.last())
	}
	// с подтверждённым доменом — реплай уходит гостю письмом
	svc.SetMailDomainVerified(true)
	bot.push(fmt.Sprintf(`{"update_id":8,"message":{"message_id":5,"text":"Есть овсяное и миндальное","chat":{"id":555,"type":"private"},"reply_to_message":{"message_id":%d}}}`, siteMsg))
	_ = svc.PollTelegram(ctx, 0)
	if m := e.email.sent[len(e.email.sent)-1]; m.Kind != "answer" || !strings.Contains(m.Body, "миндальное") {
		t.Fatalf("письмо гостю по реплаю: %+v", m)
	}
	if !strings.Contains(bot.last()["text"].(string), "письмом на anna@example.com") {
		t.Fatalf("подтверждение: %+v", bot.last())
	}
	l, _ := svc.ListQuestions(ctx)
	for _, x := range l.Items {
		if x.ID == sq.ID && x.AnsweredVia != "email" {
			t.Fatalf("вопрос с сайта не отмечен: %+v", x)
		}
	}

	// стикер — просим текст
	bot.push(`{"update_id":9,"message":{"message_id":13,"chat":{"id":888,"type":"private","first_name":"Петя"}}}`)
	_ = svc.PollTelegram(ctx, 0)
	if !strings.Contains(bot.last()["text"].(string), "только текст") {
		t.Fatalf("не текст: %+v", bot.last())
	}

	// фоновый опрос: пока он идёт, проверка из кабинета getUpdates не зовёт
	pctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { svc.RunTelegram(pctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	if _, err := svc.TelegramCheck(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
}
