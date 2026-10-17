package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Telegram — сообщения через Telegram Bot API (https://core.telegram.org/bots/api).
// Подключение чата — без вебхука: владелица жмёт «Старт» по ссылке t.me/<бот>?start=<код>,
// сервер забирает chat_id через getUpdates (бесплатный Render засыпает, вебхук бы терял сообщения).
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
	if chatID == "" {
		return fmt.Errorf("telegram: чат не подключён")
	}
	text := m.Title + "\n" + m.Body
	if m.Phone != "" && !strings.Contains(m.Body, m.Phone) {
		text += "\n" + m.Phone
	}
	in := map[string]any{"chat_id": chatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}}
	if b := t.buttons(m); b != nil {
		in["reply_markup"] = map[string]any{"inline_keyboard": b}
	}
	return t.call(ctx, "sendMessage", in, nil)
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

// Start — кто нажал «Старт» с кодом: /start <код>.
type Start struct {
	UpdateID int64
	ChatID   int64
	Name     string // @username или имя — для показа в кабинете
	Code     string
}

// Starts — новые сообщения «/start <код>» с offset; next — offset для следующего вызова
// (всё до него Telegram считает прочитанным).
func (t Telegram) Starts(ctx context.Context, offset int64) (starts []Start, next int64, err error) {
	var ups []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			Text string `json:"text"`
			Chat struct {
				ID        int64  `json:"id"`
				Type      string `json:"type"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"chat"`
		} `json:"message"`
	}
	if err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 0, "allowed_updates": []string{"message"}}, &ups); err != nil {
		return nil, offset, err
	}
	next = offset
	for _, u := range ups {
		if u.UpdateID >= next {
			next = u.UpdateID + 1
		}
		if u.Message == nil || u.Message.Chat.Type != "private" {
			continue
		}
		code, ok := strings.CutPrefix(strings.TrimSpace(u.Message.Text), "/start ")
		if !ok {
			continue
		}
		name := u.Message.Chat.FirstName
		if u.Message.Chat.Username != "" {
			name = "@" + u.Message.Chat.Username
		}
		starts = append(starts, Start{UpdateID: u.UpdateID, ChatID: u.Message.Chat.ID, Name: name, Code: strings.TrimSpace(code)})
	}
	return starts, next, nil
}
