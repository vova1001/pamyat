package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Telegram — сообщения через Telegram Bot API (https://core.telegram.org/bots/api).
// Без вебхука: сервер сам читает getUpdates в фоне (длинный опрос) — подключение владелицы («Старт» по ссылке
// t.me/<бот>?start=<код>), вопросы гостей и реплаи владелицы с ответами.
type Telegram struct {
	Token     string
	PublicURL string // адрес сайта: кнопки «Позвонить» и «Открыть кабинет» (только https — так требует Telegram)
	BaseURL   string // для тестов; по умолчанию https://api.telegram.org
	Client    *http.Client
}

func (Telegram) Name() string { return "telegram" }

type tgResp struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func (t Telegram) call(ctx context.Context, method string, in any, out any) error {
	base := t.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	body, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/bot"+t.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client(t.Client).Do(req)
	if err != nil {
		// в ошибке http-клиента — адрес с токеном: не пишем его в лог
		return fmt.Errorf("telegram %s: запрос не прошёл", method)
	}
	defer res.Body.Close()
	var r tgResp
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram %s: %s", method, res.Status)
	}
	if !r.OK {
		return fmt.Errorf("telegram %s: %s", method, r.Description)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

type button struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Buttons — кнопки-ссылки под сообщением. tel: Telegram в кнопках не принимает, поэтому «Позвонить» ведёт
// на /api/call/<номер> сайта, который перенаправляет на tel:.
func (t Telegram) buttons(m Message) [][]button {
	base := strings.TrimRight(t.PublicURL, "/")
	if !strings.HasPrefix(base, "https://") {
		return nil
	}
	var row []button
	if d := telDigits(m.Phone); d != "" {
		row = append(row, button{Text: "Позвонить", URL: base + "/api/call/" + url.PathEscape(d)})
	}
	if m.Link != "" && strings.HasPrefix(m.Link, "https://") {
		row = append(row, button{Text: "Открыть кабинет", URL: m.Link})
	}
	if len(row) == 0 {
		return nil
	}
	return [][]button{row}
}

func (t Telegram) Send(ctx context.Context, chatID string, m Message) error {
	_, err := t.SendID(ctx, chatID, m)
	return err
}

// SendID — как Send, но возвращает message_id отправленного сообщения (реплай владелицы на него = ответ гостю).
func (t Telegram) SendID(ctx context.Context, chatID string, m Message) (int64, error) {
	if chatID == "" {
		return 0, fmt.Errorf("telegram: чат не подключён")
	}
	text := m.Title + "\n" + m.Body
	if m.Phone != "" && !strings.Contains(m.Body, m.Phone) {
		text += "\n" + m.Phone
	}
	in := map[string]any{"chat_id": chatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}}
	if b := t.buttons(m); b != nil {
		in["reply_markup"] = map[string]any{"inline_keyboard": b}
	}
	var out struct {
		MessageID int64 `json:"message_id"`
	}
	err := t.call(ctx, "sendMessage", in, &out)
	return out.MessageID, err
}

// Reply — простой текст в чат; replyTo ≠ 0 — ответом на это сообщение (если его уже нет — просто текстом).
func (t Telegram) Reply(ctx context.Context, chatID int64, text string, replyTo int64) (int64, error) {
	in := map[string]any{"chat_id": chatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}}
	if replyTo != 0 {
		in["reply_parameters"] = map[string]any{"message_id": replyTo, "allow_sending_without_reply": true}
	}
	var out struct {
		MessageID int64 `json:"message_id"`
	}
	err := t.call(ctx, "sendMessage", in, &out)
	return out.MessageID, err
}

// BotUsername — имя бота (для ссылки t.me/<бот>).
func (t Telegram) BotUsername(ctx context.Context) (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := t.call(ctx, "getMe", map[string]any{}, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

// Update — входящее личное сообщение боту: «/start <код>» владелицы, вопрос гостя или реплай владелицы.
type Update struct {
	UpdateID  int64
	ChatID    int64
	MessageID int64
	Username  string // без @; может быть пустым
	Name      string // имя и фамилия из Telegram
	Text      string // текст или подпись к фото; пусто — стикер, голосовое и т. п.
	ReplyTo   int64  // на какое сообщение это ответ (0 — не ответ)
}

// Who — @username или имя — для показа в кабинете.
func (u Update) Who() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return u.Name
}

// StartCode — код из «/start <код>»; ok — сообщение и есть /start (код может быть пустым).
func (u Update) StartCode() (code string, ok bool) {
	t := strings.TrimSpace(u.Text)
	if t == "/start" {
		return "", true
	}
	code, ok = strings.CutPrefix(t, "/start ")
	return strings.TrimSpace(code), ok
}

// pollClient — для длинного опроса: ждёт дольше, чем Telegram держит соединение.
var pollClient = &http.Client{Timeout: 50 * time.Second}

// Updates — новые личные сообщения с offset; wait — сколько секунд Telegram ждёт новых (0 — сразу ответ).
// next — offset для следующего вызова (всё до него Telegram считает прочитанным).
func (t Telegram) Updates(ctx context.Context, offset int64, wait int) (ups []Update, next int64, err error) {
	var raw []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			MessageID int64  `json:"message_id"`
			Text      string `json:"text"`
			Caption   string `json:"caption"`
			Chat      struct {
				ID        int64  `json:"id"`
				Type      string `json:"type"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
			} `json:"chat"`
			ReplyTo *struct {
				MessageID int64 `json:"message_id"`
			} `json:"reply_to_message"`
		} `json:"message"`
	}
	tc := t
	if wait > 0 && t.Client == nil {
		tc.Client = pollClient
	}
	in := map[string]any{"offset": offset, "timeout": wait, "allowed_updates": []string{"message"}}
	if err := tc.call(ctx, "getUpdates", in, &raw); err != nil {
		return nil, offset, err
	}
	next = offset
	for _, u := range raw {
		if u.UpdateID >= next {
			next = u.UpdateID + 1
		}
		m := u.Message
		if m == nil || m.Chat.Type != "private" {
			continue
		}
		up := Update{UpdateID: u.UpdateID, ChatID: m.Chat.ID, MessageID: m.MessageID, Username: m.Chat.Username,
			Name: strings.TrimSpace(m.Chat.FirstName + " " + m.Chat.LastName), Text: strings.TrimSpace(m.Text)}
		if up.Text == "" {
			up.Text = strings.TrimSpace(m.Caption)
		}
		if m.ReplyTo != nil {
			up.ReplyTo = m.ReplyTo.MessageID
		}
		ups = append(ups, up)
	}
	return ups, next, nil
}
