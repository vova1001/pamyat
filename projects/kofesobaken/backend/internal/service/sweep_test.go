package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
)

// checkProvider — тестовая оплата, которая ещё и отвечает о статусе (как ЮKassa), без уведомлений.
type checkProvider struct {
	*payments.TestProvider
	status map[string]string // provider_payment_id → succeeded | canceled; нет — ещё ждёт оплаты
	amount map[string]int64
	ids    map[string]uuid.UUID
	err    error // сбой связи
	calls  int
}

func (c *checkProvider) Create(ctx context.Context, req payments.CreateRequest) (payments.CreateResult, error) {
	r, err := c.TestProvider.Create(ctx, req)
	c.amount[r.ProviderPaymentID], c.ids[r.ProviderPaymentID] = req.AmountKop, req.PaymentID
	return r, err
}

func (c *checkProvider) Check(_ context.Context, id string) (payments.Event, error) {
	c.calls++
	if c.err != nil {
		return payments.Event{}, c.err
	}
	t := map[string]string{"succeeded": payments.EventSucceeded, "canceled": payments.EventCanceled}[c.status[id]]
	if t == "" {
		return payments.Event{}, payments.ErrIgnored
	}
	return payments.Event{Type: t, PaymentID: c.ids[id], ProviderPaymentID: id, AmountKop: c.amount[id]}, nil
}

func newSweepEnv(t *testing.T) (*env, *checkProvider, *service.Service) {
	e := newEnv(t)
	cp := &checkProvider{TestProvider: payments.NewTestProvider("http://site.test", "", "secret"),
		status: map[string]string{}, amount: map[string]int64{}, ids: map[string]uuid.UUID{}}
	return e, cp, service.New(testPool, cp, []notify.Channel{e.email, e.tg}, msk)
}

func TestPaymentSweep(t *testing.T) {
	e, cp, svc := newSweepEnv(t)
	ctx := context.Background()
	b, err := svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(1), Time: "18:00", Guests: 2})
	if err != nil {
		t.Fatal(err)
	}
	o, err := svc.CreateOrder(ctx, service.OrderInput{Mode: "pickup", Phone: "9001112233", Items: []service.OrderItemInput{{MenuItemID: menuID(t, e, "Чай"), Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	prov := func(id uuid.UUID) string { return "test_" + id.String() }

	// Ещё не оплачено — ничего не меняется, в кабинете «ждёт оплаты».
	if n, err := svc.SweepPayments(ctx, false); err != nil || n != 0 {
		t.Fatalf("ждут оплаты: %d %v", n, err)
	}
	if l, _ := svc.ListBookings(ctx); len(l) != 1 || l[0].Status != "awaiting_payment" {
		t.Fatalf("бронь «ждёт оплаты»: %+v", l)
	}

	// Гостья оплатила и не вернулась, уведомления нет: проверка применяет оплату.
	cp.status[prov(b.PaymentID)] = "succeeded"
	cp.status[prov(o.PaymentID)] = "canceled"
	if n, err := svc.SweepPayments(ctx, false); err != nil || n != 2 {
		t.Fatalf("проверка: %d %v", n, err)
	}
	if l, _ := svc.ListBookings(ctx); len(l) != 1 || l[0].Status != "new" {
		t.Fatalf("оплаченная бронь в кабинете: %+v", l)
	}
	if len(e.email.sent) != 1 || e.email.sent[0].Title != "Новая бронь столика" {
		t.Fatalf("письмо о брони: %+v", e.email.sent)
	}
	if p, _ := svc.Payment(ctx, o.PaymentID); p.Status != "canceled" {
		t.Fatalf("отменённый платёж: %+v", p)
	}
	if l, _ := svc.ListOrders(ctx); len(l) != 0 {
		t.Fatalf("заказ без оплаты закрыт и скрыт: %+v", l)
	}

	// Повтор (и уведомление после проверки) ничего не меняет — письмо не дублируется.
	if n, _ := svc.SweepPayments(ctx, true); n != 0 {
		t.Fatalf("повтор: %d", n)
	}
	ev, _ := cp.Check(ctx, prov(b.PaymentID))
	if err := svc.HandlePaymentEvent(ctx, ev); err != nil || len(e.email.sent) != 1 {
		t.Fatalf("повторное уведомление: %v, писем %d", err, len(e.email.sent))
	}
}

func TestPaymentSweepWindows(t *testing.T) {
	_, cp, svc := newSweepEnv(t)
	ctx := context.Background()
	b, err := svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(1), Time: "18:00", Guests: 2})
	if err != nil {
		t.Fatal(err)
	}

	// Через 3 часа платёж уже не в ежеминутной проверке, но в 10-минутной.
	svc.SetClock(func() time.Time { return time.Now().Add(3 * time.Hour) })
	cp.calls = 0
	if n, _ := svc.SweepPayments(ctx, false); n != 0 || cp.calls != 0 {
		t.Fatalf("ежеминутная — только моложе 2 часов: %d, запросов %d", n, cp.calls)
	}
	if n, _ := svc.SweepPayments(ctx, true); n != 0 || cp.calls != 1 {
		t.Fatalf("10-минутная — до суток: %d, запросов %d", n, cp.calls)
	}

	// Старше суток, ЮKassa недоступна — не трогаем, попробуем позже.
	svc.SetClock(func() time.Time { return time.Now().Add(25 * time.Hour) })
	cp.err = errors.New("сеть")
	if n, _ := svc.SweepPayments(ctx, true); n != 0 {
		t.Fatalf("сбой связи — без изменений: %d", n)
	}
	// Старше суток и не оплачен — закрываем как просроченный.
	cp.err = nil
	if n, _ := svc.SweepPayments(ctx, true); n != 1 {
		t.Fatalf("просроченный: %d", n)
	}
	if p, _ := svc.Payment(ctx, b.PaymentID); p.Status != "canceled" {
		t.Fatalf("просроченный платёж закрыт: %+v", p)
	}
	if l, _ := svc.ListBookings(ctx); len(l) != 0 {
		t.Fatalf("просроченная бронь скрыта: %+v", l)
	}
}

func TestPaymentSweepLateSuccess(t *testing.T) {
	_, cp, svc := newSweepEnv(t)
	ctx := context.Background()
	b, err := svc.CreateBooking(ctx, service.BookingInput{Name: "Анна", Phone: "9001112233", Date: day(1), Time: "18:00", Guests: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Старше суток, но в ЮKassa оплачен — применяем оплату, а не закрываем.
	svc.SetClock(func() time.Time { return time.Now().Add(25 * time.Hour) })
	cp.status["test_"+b.PaymentID.String()] = "succeeded"
	if n, _ := svc.SweepPayments(ctx, true); n != 1 {
		t.Fatalf("поздняя оплата: %d", n)
	}
	if p, _ := svc.Payment(ctx, b.PaymentID); p.Status != "succeeded" {
		t.Fatalf("оплачен: %+v", p)
	}
}
