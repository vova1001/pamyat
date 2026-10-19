package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/store"
)

// Вопросы гостей. Приходят из формы на сайте (ответ — на почту или в Telegram по нику) и сообщениями боту
// (ответ — тем же ботом). Владелица видит их в кабинете, на почте (Reply-To = почта гостя) и в Telegram;
// отвечает из кабинета или реплаем в Telegram.

const (
	QuestionNameMax   = 60
	QuestionBodyMax   = 2000
	QuestionAnswerMax = 3500 // с подписью влезает в одно сообщение Telegram (4096)
	questionsListMax  = 300
	guestAckPause     = 30 * time.Minute // «Спасибо, вопрос передали» — не на каждое сообщение подряд
)

// QuestionInput — форма «Задать вопрос» на сайте.
type QuestionInput struct {
	Name     string `json:"name"`
	Via      string `json:"via"`     // email | telegram — куда ответить
	Contact  string `json:"contact"` // почта или @ник
	Question string `json:"question"`
	Website  string `json:"website"` // ловушка для ботов: поле скрыто, человек его не заполнит
}

// Question — вопрос для кабинета.
type Question struct {
	ID          int64      `json:"id"`
	Source      string     `json:"source"`       // site | telegram
	Name        string     `json:"name"`         // имя гостя
	ContactKind string     `json:"contact_kind"` // email | telegram
	Contact     string     `json:"contact"`      // почта или @ник (у вопроса из бота может быть пустым)
	Body        string     `json:"body"`
	Answer      string     `json:"answer"`
	AnsweredVia string     `json:"answered_via"` // telegram | email | manual (ответили сами — письмом, в Telegram)
	AnsweredAt  *time.Time `json:"answered_at"`
	CreatedAt   time.Time  `json:"created_at"`
	CanSend     bool       `json:"can_send"` // ответ из кабинета дойдёт до гостя сам (бот или письмо с сайта)
}

// QuestionList — вопросы и что умеет отправка ответов.
type QuestionList struct {
	Items      []Question `json:"items"`
	MailDirect bool       `json:"mail_direct"` // письма гостям уходят с сайта (подтверждён домен почты)
}

// SetMailDomainVerified — у Resend подтверждён свой домен: письма доходят до любой почты, а не только владельца
// аккаунта Resend. Тогда ответ из кабинета уходит гостю письмом.
func (s *Service) SetMailDomainVerified(v bool) { s.mailDirect = v }

// mailGuests — можно ли слать письма гостям с сайта.
func (s *Service) mailGuests() (notify.Channel, bool) {
	ch, ok := s.mailer()
	return ch, ok && s.mailDirect
}

func (s *Service) channel(name string) notify.Channel {
	for _, ch := range s.channels {
		if ch.Name() == name {
			return ch
		}
	}
	return nil
}

func (s *Service) canSend(q store.Question) bool {
	if q.Source == "telegram" {
		return q.TgChatID.Valid && s.channel("telegram") != nil
	}
	_, ok := s.mailGuests()
	return q.ContactKind == "email" && ok
}

func (s *Service) toQuestion(q store.Question) Question {
	return Question{ID: q.ID, Source: q.Source, Name: q.Name, ContactKind: q.ContactKind, Contact: q.Contact, Body: q.Body,
		Answer: q.Answer, AnsweredVia: q.AnsweredVia, AnsweredAt: q.AnsweredAt, CreatedAt: q.CreatedAt, CanSend: s.canSend(q)}
}

// ValidEmail — адрес вида name@domain.tld без имени и угловых скобок.
func ValidEmail(v string) bool {
	a, err := mail.ParseAddress(v)
	if err != nil || a.Address != v || utf8.RuneCountInString(v) > 254 {
		return false
	}
	_, domain, _ := strings.Cut(v, "@")
	return strings.Contains(domain, ".")
}

// CreateQuestion — вопрос с сайта: в кабинет, владелице на почту и в Telegram.
func (s *Service) CreateQuestion(ctx context.Context, in QuestionInput) (Question, error) {
	if strings.TrimSpace(in.Website) != "" {
		return Question{}, nil // бот заполнил скрытое поле — делаем вид, что приняли
	}
	f := fields{}
	name, body, contact := cleanText(in.Name), cleanText(in.Question), cleanText(in.Contact)
	checkLen(f, "name", name, 1, QuestionNameMax, "Как к вам обращаться?")
	checkLen(f, "question", body, 3, QuestionBodyMax, fmt.Sprintf("Напишите вопрос — до %d символов", QuestionBodyMax))
	switch in.Via {
	case "email":
		contact = strings.ToLower(contact)
		if !ValidEmail(contact) {
			f.add("contact", "Проверьте почту — на неё придёт ответ")
		}
	case "telegram":
		var ok bool
		if contact, ok = NormalizeTelegram(contact); !ok {
			f.add("contact", "Ник в Telegram — латиница, цифры и _, например @anna_k")
		}
	default:
		f.add("via", "Выберите, куда прислать ответ")
	}
	if err := f.err(); err != nil {
		return Question{}, err
	}
	q, err := s.q.AddQuestion(ctx, store.AddQuestionParams{Source: "site", Name: name, ContactKind: in.Via, Contact: contact, Body: body})
	if err != nil {
		return Question{}, err
	}
	s.notifyQuestion(ctx, q)
	return s.toQuestion(q), nil
}

func (s *Service) ListQuestions(ctx context.Context) (QuestionList, error) {
	rows, err := s.q.ListQuestions(ctx, questionsListMax)
	if err != nil {
		return QuestionList{}, err
	}
	out := QuestionList{Items: make([]Question, 0, len(rows))}
	_, out.MailDirect = s.mailGuests()
	for _, r := range rows {
		out.Items = append(out.Items, s.toQuestion(r))
	}
	return out, nil
}

// AnswerQuestion сохраняет ответ. send — отправить гостю (ботом или письмом с сайта); иначе только отметка
// «отвечено» (владелица ответила сама — из своей почты или Telegram).
func (s *Service) AnswerQuestion(ctx context.Context, id int64, answer string, send bool) (Question, error) {
	answer = cleanText(answer)
	if send || answer != "" {
		f := fields{}
		checkLen(f, "answer", answer, 1, QuestionAnswerMax, fmt.Sprintf("Напишите ответ — до %d символов", QuestionAnswerMax))
		if err := f.err(); err != nil {
			return Question{}, err
		}
	}
	q, err := s.q.GetQuestion(ctx, id)
	if err != nil {
		return Question{}, notFound(err)
	}
	via := "manual"
	if send {
		if via, err = s.deliverAnswer(ctx, q, answer); err != nil {
			return Question{}, err
		}
	}
	q, err = s.q.AnswerQuestion(ctx, store.AnswerQuestionParams{ID: id, Answer: answer, AnsweredVia: via})
	if err != nil {
		return Question{}, err
	}
	return s.toQuestion(q), nil
}

const answerTitle = "Ответ на ваш вопрос — КофеСобакен"

// deliverAnswer отправляет ответ гостю; пишет в журнал уведомлений.
func (s *Service) deliverAnswer(ctx context.Context, q store.Question, answer string) (string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	var (
		ch  notify.Channel
		to  string
		m   notify.Message
		err error
	)
	switch {
	case q.Source == "telegram" && q.TgChatID.Valid:
		ch, to = s.channel("telegram"), strconv.FormatInt(q.TgChatID.Int64, 10)
		m = notify.Message{Kind: "answer", Title: "Ответ от КофеСобакен", Body: answer}
		if ch == nil {
			return "", &ConflictError{Msg: "Telegram не настроен — ответ не отправлен"}
		}
		if tg, ok := ch.(notify.Telegram); ok {
			_, err = tg.Reply(ctx, q.TgChatID.Int64, m.Title+":\n\n"+answer, q.TgMessageID.Int64)
		} else {
			err = ch.Send(ctx, to, m) // без бота — заглушка, сообщение в лог
		}
		if err != nil {
			logErr("ответ гостю в Telegram", err, "question", q.ID)
			err = &ConflictError{Msg: "Telegram не принял ответ — возможно, гость удалил чат с ботом. Ответ не сохранён"}
		}
	case q.ContactKind == "email":
		var ok bool
		if ch, ok = s.mailGuests(); !ok {
			return "", &ConflictError{Msg: "Письмо с сайта пока не уходит гостям — нажмите «Ответить письмом»"}
		}
		ns, nerr := s.notifySettings(ctx)
		if nerr != nil {
			return "", nerr
		}
		to = q.Contact
		m = notify.Message{Kind: "answer", Title: answerTitle, Body: answer + "\n\nВаш вопрос:\n" + q.Body, ReplyTo: ns.Email}
		if err = ch.Send(ctx, to, m); err != nil {
			logErr("ответ гостю письмом", err, "question", q.ID)
			err = &ConflictError{Msg: "Письмо не ушло — попробуйте ещё раз или нажмите «Ответить письмом»"}
		}
	default:
		return "", &ConflictError{Msg: "Гость оставил ник — напишите ему в Telegram сами и нажмите «Отметить отвеченным»"}
	}
	status := "sent"
	if err != nil {
		status = "failed"
	}
	if _, jerr := s.q.AddNotification(ctx, store.AddNotificationParams{Channel: ch.Name(), Kind: m.Kind, Recipient: to, Title: m.Title, Body: answer, Status: status}); jerr != nil {
		logErr("журнал уведомлений", jerr)
	}
	return ch.Name(), err
}

// questionHow — как ответить гостю (подсказка в сообщении владелице).
func (s *Service) questionHow(q store.Question, inTelegram bool) string {
	switch {
	case inTelegram && s.canSend(q):
		return "Ответьте на это сообщение (реплаем) — ответ уйдёт гостю. Или ответьте в кабинете."
	case q.Source == "telegram":
		return "Ответьте в кабинете — бот перешлёт ответ гостю."
	case q.ContactKind == "email" && !inTelegram:
		return "Нажмите «Ответить» в почте — письмо уйдёт гостю на " + q.Contact + "."
	case q.ContactKind == "email":
		return "Ответьте гостю письмом на " + q.Contact + " (в почте — письмо с вопросом, «Ответить» — сразу ему)."
	default:
		return "Напишите гостю в Telegram: https://t.me/" + strings.TrimPrefix(q.Contact, "@")
	}
}

// notifyQuestion — вопрос владелице: на почту (если включена) и в Telegram (если подключён).
// В Telegram запоминаем id сообщения: реплай на него — ответ гостю.
func (s *Service) notifyQuestion(ctx context.Context, q store.Question) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	ns, err := s.notifySettings(ctx)
	if err != nil {
		logErr("настройки уведомлений", err)
		return
	}
	title := "Вопрос с сайта"
	if q.Source == "telegram" {
		title = "Вопрос из Telegram"
	}
	who := q.Name
	if q.Contact != "" && q.Contact != q.Name {
		who += " · " + q.Contact
	}
	for _, ch := range s.channels {
		m := notify.Message{Kind: "question", Title: title, Link: s.cabinet("questions")}
		var to string
		switch ch.Name() {
		case "email":
			if !ns.EmailEnabled || ns.Email == "" {
				continue
			}
			to = ns.Email
			if q.ContactKind == "email" {
				m.ReplyTo = q.Contact
			}
			m.Body = who + "\n\n" + q.Body + "\n\n" + s.questionHow(q, false)
		case "telegram":
			if !ns.TelegramConnected {
				continue
			}
			to = ns.tgRecipient()
			m.Body = who + "\n\n" + q.Body + "\n\n" + s.questionHow(q, true)
		default:
			continue
		}
		status := "sent"
		if tg, ok := ch.(notify.Telegram); ok {
			id, err := tg.SendID(ctx, to, m)
			if err == nil && id != 0 {
				err = s.q.SetQuestionOwnerMessage(ctx, store.SetQuestionOwnerMessageParams{ID: q.ID, OwnerMessageID: pgtype.Int8{Int64: id, Valid: true}})
			}
			if err != nil {
				logErr("вопрос в Telegram", err, "question", q.ID)
				status = "failed"
			}
		} else if err := ch.Send(ctx, to, m); err != nil {
			logErr("вопрос владелице", err, "channel", ch.Name(), "question", q.ID)
			status = "failed"
		}
		if _, err := s.q.AddNotification(ctx, store.AddNotificationParams{Channel: ch.Name(), Kind: m.Kind, Recipient: to, Title: m.Title, Body: m.Body, Status: status}); err != nil {
			logErr("журнал уведомлений", err)
		}
	}
}

// ---------- входящие сообщения боту ----------

const (
	msgGuestHello  = "Здравствуйте! Это бот кофейни КофеСобакен. Напишите свой вопрос — ответим здесь же, в этом чате."
	msgGuestThanks = "Спасибо! Вопрос передали — ответ придёт сюда, в этот чат."
	msgGuestText   = "Пока понимаю только текст — напишите вопрос словами, пожалуйста."
	msgOwnerHow    = "Чтобы ответить гостю, ответьте на сообщение с его вопросом (реплаем: свайп влево или «Ответить»)."
)

// handleUpdate — одно входящее: «/start <код>» владелицы, реплай владелицы с ответом, вопрос гостя.
// Ошибка — только если не удалось записать в базу (сообщение разберём ещё раз).
func (s *Service) handleUpdate(ctx context.Context, tg notify.Telegram, u notify.Update) error {
	ns, err := s.notifySettings(ctx)
	if err != nil {
		return err
	}
	owner := ns.TelegramChatID != "" && ns.TelegramChatID == strconv.FormatInt(u.ChatID, 10)
	if code, ok := u.StartCode(); ok {
		if code != "" {
			link, err := getJSON(ctx, s.q, keyTgLink, tgLink{})
			if err != nil {
				return err
			}
			if link.CodeHash != "" && !s.now().After(link.ExpiresAt) && hashToken(code) == link.CodeHash {
				return s.connectOwner(ctx, u, link)
			}
			return nil // старая ссылка подключения — молча
		}
		if !owner {
			s.botSay(ctx, tg, u.ChatID, msgGuestHello, 0)
		}
		return nil
	}
	if owner {
		return s.ownerReply(ctx, tg, u)
	}
	return s.guestQuestion(ctx, tg, u)
}

func (s *Service) botSay(ctx context.Context, tg notify.Telegram, chat int64, text string, replyTo int64) {
	if _, err := tg.Reply(ctx, chat, text, replyTo); err != nil {
		logErr("сообщение бота", err)
	}
}

func (s *Service) guestQuestion(ctx context.Context, tg notify.Telegram, u notify.Update) error {
	if u.Text == "" {
		s.botSay(ctx, tg, u.ChatID, msgGuestText, u.MessageID)
		return nil
	}
	body := u.Text
	if utf8.RuneCountInString(body) > QuestionBodyMax {
		body = string([]rune(body)[:QuestionBodyMax]) + "…"
	}
	name := u.Name
	if name == "" {
		name = u.Who()
	}
	if name == "" {
		name = "Гость"
	}
	contact := ""
	if u.Username != "" {
		contact = "@" + u.Username
	}
	q, err := s.q.AddQuestion(ctx, store.AddQuestionParams{Source: "telegram", Name: name, ContactKind: "telegram", Contact: contact, Body: body,
		TgChatID: pgtype.Int8{Int64: u.ChatID, Valid: true}, TgMessageID: pgtype.Int8{Int64: u.MessageID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // это сообщение уже разобрано
	}
	if err != nil {
		return err
	}
	prev, err := s.q.LastQuestionFromChat(ctx, store.LastQuestionFromChatParams{TgChatID: q.TgChatID, ID: q.ID})
	ack := errors.Is(err, pgx.ErrNoRows) || err == nil && s.now().Sub(prev) > guestAckPause
	s.notifyQuestion(ctx, q)
	if ack {
		s.botSay(ctx, tg, u.ChatID, msgGuestThanks, u.MessageID)
	}
	return nil
}

func (s *Service) ownerReply(ctx context.Context, tg notify.Telegram, u notify.Update) error {
	if u.ReplyTo == 0 {
		s.botSay(ctx, tg, u.ChatID, msgOwnerHow, u.MessageID)
		return nil
	}
	q, err := s.q.QuestionByOwnerMessage(ctx, pgtype.Int8{Int64: u.ReplyTo, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		s.botSay(ctx, tg, u.ChatID, "Это не сообщение с вопросом гостя. "+msgOwnerHow, u.MessageID)
		return nil
	}
	if err != nil {
		return err
	}
	if u.Text == "" {
		s.botSay(ctx, tg, u.ChatID, "Ответ — только текстом.", u.MessageID)
		return nil
	}
	if !s.canSend(q) {
		s.botSay(ctx, tg, u.ChatID, "Отсюда гостю не отправить. "+s.questionHow(q, false), u.MessageID)
		return nil
	}
	_, err = s.AnswerQuestion(ctx, q.ID, u.Text, true)
	var ce *ConflictError
	var ve *ValidationError
	switch {
	case errors.As(err, &ce):
		s.botSay(ctx, tg, u.ChatID, ce.Msg, u.MessageID)
	case errors.As(err, &ve):
		s.botSay(ctx, tg, u.ChatID, fmt.Sprintf("Слишком длинный ответ — до %d символов.", QuestionAnswerMax), u.MessageID)
	case err != nil:
		return err
	case q.Source == "telegram":
		s.botSay(ctx, tg, u.ChatID, "Отправлено гостю.", u.MessageID)
	default:
		s.botSay(ctx, tg, u.ChatID, "Отправлено гостю письмом на "+q.Contact+".", u.MessageID)
	}
	return nil
}
