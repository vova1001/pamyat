package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
)

// DefaultMailFrom — отправитель Resend до подтверждения своего домена (письма доходят только на почту
// владельца аккаунта Resend).
const DefaultMailFrom = "КофеСобакен <onboarding@resend.dev>"

// Resend — почта через HTTP API Resend (https://resend.com/docs/api-reference/emails/send-email).
type Resend struct {
	APIKey  string
	From    string
	BaseURL string // для тестов; по умолчанию https://api.resend.com
	Client  *http.Client
}

func (Resend) Name() string { return "email" }

func (r Resend) Send(ctx context.Context, to string, m Message) error {
	if to == "" {
		return fmt.Errorf("resend: нет адреса получателя")
	}
	from := r.From
	if from == "" {
		from = DefaultMailFrom
	}
	base := r.BaseURL
	if base == "" {
		base = "https://api.resend.com"
	}
	body := map[string]any{
		"from":    from,
		"to":      []string{to},
		"subject": m.Title,
		"html":    EmailHTML(m),
		"text":    emailText(m),
	}
	if m.ReplyTo != "" {
		body["reply_to"] = m.ReplyTo
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := client(r.Client).Do(req)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("resend: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func linkText(m Message) string {
	switch m.Kind {
	case "reset", "setup":
		return "Задать пароль"
	}
	return "Открыть кабинет"
}

// EmailHTML — простое письмо: заголовок, строки сообщения, телефон ссылкой, кнопка-ссылка.
func EmailHTML(m Message) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="margin:0;padding:24px;background:#f6f1ea;font-family:Arial,sans-serif;color:#2b211b">`)
	b.WriteString(`<div style="max-width:520px;margin:0 auto;background:#fff;border-radius:16px;padding:24px">`)
	fmt.Fprintf(&b, `<h2 style="margin:0 0 16px;font-size:20px">%s</h2>`, html.EscapeString(m.Title))
	for _, line := range strings.Split(m.Body, "\n") {
		fmt.Fprintf(&b, `<p style="margin:0 0 8px;font-size:16px;line-height:1.4">%s</p>`, html.EscapeString(line))
	}
	if m.Phone != "" {
		fmt.Fprintf(&b, `<p style="margin:16px 0 0"><a href="tel:%s" style="color:#b5552b">Позвонить %s</a></p>`, html.EscapeString(telDigits(m.Phone)), html.EscapeString(m.Phone))
	}
	if m.Link != "" {
		fmt.Fprintf(&b, `<p style="margin:20px 0 0"><a href="%s" style="display:inline-block;background:#b5552b;color:#fff;text-decoration:none;padding:12px 20px;border-radius:12px">%s</a></p>`, html.EscapeString(m.Link), linkText(m))
	}
	b.WriteString(`<p style="margin:24px 0 0;font-size:12px;color:#8a7d72">Сайт кофейни КофеСобакен</p></div></body></html>`)
	return b.String()
}

func emailText(m Message) string {
	s := m.Title + "\n\n" + m.Body
	if m.Phone != "" {
		s += "\nТелефон: " + m.Phone
	}
	if m.Link != "" {
		s += "\n\n" + linkText(m) + ": " + m.Link
	}
	return s
}

// telDigits — номер для tel: — только цифры и ведущий «+».
func telDigits(p string) string {
	var b strings.Builder
	for i, r := range p {
		if r >= '0' && r <= '9' || r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
