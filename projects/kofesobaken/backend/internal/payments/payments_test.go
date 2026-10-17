package payments

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestTestProvider(t *testing.T) {
	p := NewTestProvider("https://site.ru", "http://127.0.0.1:1/hook", "s3cret")
	id := uuid.New()
	r, err := p.Create(context.Background(), CreateRequest{PaymentID: id, AmountKop: 50000})
	if err != nil || r.ConfirmationURL != "https://site.ru/pay/"+id.String() || !strings.HasPrefix(r.ProviderPaymentID, "test_") {
		t.Fatalf("Create: %v %+v", err, r)
	}
	body := []byte(`{"event":"payment.succeeded","payment_id":"` + id.String() + `","provider_payment_id":"p","amount_kop":50000}`)
	h := http.Header{}
	h.Set("X-Signature", Sign([]byte("s3cret"), body))
	e, err := p.ParseWebhook(h, body)
	if err != nil || e.PaymentID != id || e.AmountKop != 50000 {
		t.Fatalf("ParseWebhook: %v %+v", err, e)
	}
	h.Set("X-Signature", Sign([]byte("other"), body))
	if _, err := p.ParseWebhook(h, body); err != ErrBadSignature {
		t.Fatal("чужая подпись не принимается")
	}
	bad := []byte(`{"event":"payment.weird"}`)
	h.Set("X-Signature", Sign([]byte("s3cret"), bad))
	if _, err := p.ParseWebhook(h, bad); err == nil {
		t.Fatal("неизвестное событие не принимается")
	}
	if err := p.Refund(context.Background(), "p", 100); err != nil {
		t.Fatal(err)
	}
	if err := p.Simulate(context.Background(), EventSucceeded, id, "p", 1); err == nil {
		t.Fatal("недоступный webhook — ошибка")
	}
	if p.Name() != "test" {
		t.Fatal("имя")
	}
}
