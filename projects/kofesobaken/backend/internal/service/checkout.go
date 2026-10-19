package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/store"
)

// Checkout — результат оформления: что создано и куда отправить гостя оплачивать.
type Checkout struct {
	ID              int64     `json:"id"`
	Kind            string    `json:"kind"`
	AmountKop       int64     `json:"amount_kop"`
	Status          string    `json:"status"` // awaiting_payment | confirmed
	PaymentID       uuid.UUID `json:"payment_id,omitempty"`
	ConfirmationURL string    `json:"confirmation_url,omitempty"`
}

// createPayment создаёт платёж в БД (внутри транзакции оформления).
func createPayment(ctx context.Context, q *store.Queries, provider, kind string, target, amount int64, desc string) (store.Payment, error) {
	return q.CreatePayment(ctx, store.CreatePaymentParams{Kind: kind, TargetID: target, AmountKop: amount, Description: desc, Provider: provider})
}

// startPayment регистрирует платёж в платёжной системе и возвращает ссылку на оплату.
// phone — телефон гостя, если есть (для чека, когда его отправляет платёжная система).
func (s *Service) startPayment(ctx context.Context, p store.Payment, c *Checkout, phone string) error {
	back := map[string]string{"booking": "#booking", "order": "#order", "signup": "#events"}[p.Kind]
	res, err := s.pay.Create(ctx, payments.CreateRequest{
		PaymentID: p.ID, AmountKop: p.AmountKop, Description: p.Description, Phone: phone,
		ReturnURL: s.publicURL + "/?payment=" + p.ID.String() + back,
	})
	if err != nil {
		return fmt.Errorf("создание платежа: %w", err)
	}
	if err := s.q.SetPaymentProviderID(ctx, store.SetPaymentProviderIDParams{ID: p.ID, ProviderPaymentID: res.ProviderPaymentID}); err != nil {
		return err
	}
	c.PaymentID, c.ConfirmationURL = p.ID, res.ConfirmationURL
	return nil
}

// ---------- заказ с собой / доставка ----------

type OrderItemInput struct {
	MenuItemID int64 `json:"menu_item_id"`
	Qty        int   `json:"qty"`
}

type OrderInput struct {
	Mode    string           `json:"mode"`
	Phone   string           `json:"phone"`
	Address string           `json:"address"`
	Items   []OrderItemInput `json:"items"`
}

func (s *Service) CreateOrder(ctx context.Context, in OrderInput) (Checkout, error) {
	f := fields{}
	if in.Mode != "pickup" && in.Mode != "delivery" {
		f.add("mode", "Выберите: заберу сам или доставка")
	}
	phone, ok := NormalizePhone(in.Phone)
	if !ok {
		f.add("phone", "Укажите телефон")
	}
	in.Address = cleanText(in.Address)
	if in.Mode == "delivery" {
		checkLen(f, "address", in.Address, 5, 200, "Укажите адрес доставки")
	} else {
		in.Address = ""
	}
	if len(in.Items) == 0 {
		f.add("items", "Сначала добавьте напитки из меню")
	}
	if len(in.Items) > 30 {
		f.add("items", "Слишком много позиций")
	}
	qty := map[int64]int{}
	var ids []int64
	for _, it := range in.Items {
		if it.Qty < 1 || it.Qty > 20 {
			f.add("items", "Количество — от 1 до 20")
			continue
		}
		if _, seen := qty[it.MenuItemID]; !seen {
			ids = append(ids, it.MenuItemID)
		}
		qty[it.MenuItemID] += it.Qty
	}
	if err := f.err(); err != nil {
		return Checkout{}, err
	}
	menu, err := s.q.GetMenuItemsByIDs(ctx, ids)
	if err != nil {
		return Checkout{}, err
	}
	byID := map[int64]store.MenuItem{}
	for _, m := range menu {
		byID[m.ID] = m
	}
	var total int64
	for _, id := range ids {
		m, ok := byID[id]
		if !ok || !m.Visible {
			return Checkout{}, &ConflictError{Msg: "Одной из позиций уже нет в меню — обновите страницу"}
		}
		total += m.PriceKop * int64(qty[id])
	}
	if in.Mode == "delivery" {
		os, err := s.orderSettings(ctx)
		if err != nil {
			return Checkout{}, err
		}
		if total < os.DeliveryMinKop {
			return Checkout{}, &ValidationError{Fields: map[string]string{"mode": fmt.Sprintf("Доставка — от %s. Добавьте ещё %s", Rub(os.DeliveryMinKop), Rub(os.DeliveryMinKop-total))}}
		}
	}

	var c Checkout
	var p store.Payment
	err = s.inTx(ctx, func(q *store.Queries) error {
		o, err := q.CreateOrder(ctx, store.CreateOrderParams{Mode: in.Mode, Phone: phone, Address: in.Address, TotalKop: total, DeliveryKop: 0})
		if err != nil {
			return err
		}
		for _, id := range ids {
			m := byID[id]
			if err := q.AddOrderItem(ctx, store.AddOrderItemParams{OrderID: o.ID, MenuItemID: pgtype.Int8{Int64: m.ID, Valid: true}, Name: m.Name, PriceKop: m.PriceKop, Qty: int32(qty[id])}); err != nil {
				return err
			}
		}
		p, err = createPayment(ctx, q, s.pay.Name(), "order", o.ID, total, fmt.Sprintf("Заказ №%d, КофеСобакен", o.ID))
		c = Checkout{ID: o.ID, Kind: "order", AmountKop: total, Status: "awaiting_payment"}
		return err
	})
	if err != nil {
		return Checkout{}, err
	}
	return c, s.startPayment(ctx, p, &c, phone)
}

// ---------- бронь столика ----------

type BookingInput struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Date    string `json:"date"`
	Time    string `json:"time"`
	Guests  int    `json:"guests"`
	WithDog bool   `json:"with_dog"`
}

// Часы брони: кофейня работает 9:00–21:00, последняя бронь — 20:30.
const (
	bookingFirstMin = 9 * 60
	bookingLastMin  = 20*60 + 30
	bookingDaysMax  = 60
)

func (s *Service) CreateBooking(ctx context.Context, in BookingInput) (Checkout, error) {
	f := fields{}
	in.Name = cleanText(in.Name)
	checkLen(f, "name", in.Name, 1, 60, "Укажите имя")
	phone, ok := NormalizePhone(in.Phone)
	if !ok {
		f.add("phone", "Укажите телефон")
	}
	day, okD := ParseDate(in.Date)
	at, okT := ParseClock(in.Time)
	today := s.today()
	switch {
	case !okD:
		f.add("date", "Выберите дату")
	case day.Before(today):
		f.add("date", "Эта дата уже прошла")
	case day.After(today.AddDate(0, 0, bookingDaysMax)):
		f.add("date", "Бронируем не дальше чем на 60 дней вперёд")
	}
	if !okT {
		f.add("time", "Выберите время")
	} else if m := clockMinutes(at); m < bookingFirstMin || m > bookingLastMin {
		f.add("time", "Бронь — с 9:00 до 20:30")
	} else if okD && day.Equal(today) {
		n := s.now().In(s.loc)
		if m <= n.Hour()*60+n.Minute() {
			f.add("time", "Это время уже прошло")
		}
	}
	if in.Guests < 1 || in.Guests > 10 {
		f.add("guests", "Гостей — от 1 до 10")
	}
	if err := f.err(); err != nil {
		return Checkout{}, err
	}
	var c Checkout
	var p store.Payment
	err := s.inTx(ctx, func(q *store.Queries) error {
		b, err := q.CreateBooking(ctx, store.CreateBookingParams{Name: in.Name, Phone: phone, Day: day, At: at, Guests: int32(in.Guests), WithDog: in.WithDog, PrepayKop: BookingPrepayKop})
		if err != nil {
			return err
		}
		p, err = createPayment(ctx, q, s.pay.Name(), "booking", b.ID, BookingPrepayKop, fmt.Sprintf("Предоплата брони столика №%d, КофеСобакен", b.ID))
		c = Checkout{ID: b.ID, Kind: "booking", AmountKop: BookingPrepayKop, Status: "awaiting_payment"}
		return err
	})
	if err != nil {
		return Checkout{}, err
	}
	return c, s.startPayment(ctx, p, &c, phone)
}

// ---------- запись на мероприятие ----------

type SignupInput struct {
	Name    string `json:"name"`
	Via     string `json:"via"`
	Contact string `json:"contact"`
	People  int    `json:"people"`
	WithDog bool   `json:"with_dog"`
}

func (s *Service) CreateSignup(ctx context.Context, eventID int64, in SignupInput) (Checkout, error) {
	f := fields{}
	in.Name = cleanText(in.Name)
	checkLen(f, "name", in.Name, 1, 60, "Укажите имя")
	var contact string
	var ok bool
	switch in.Via {
	case "phone":
		contact, ok = NormalizePhone(in.Contact)
		if !ok {
			f.add("contact", "Укажите телефон")
		}
	case "tg":
		contact, ok = NormalizeTelegram(in.Contact)
		if !ok {
			f.add("contact", "Укажите ник в Telegram, например @anna")
		}
	default:
		f.add("via", "Выберите, как с вами связаться")
	}
	if in.People < 1 || in.People > 6 {
		f.add("people", "От 1 до 6 человек")
	}
	if err := f.err(); err != nil {
		return Checkout{}, err
	}

	var c Checkout
	var p store.Payment
	var free bool
	var su store.EventSignup
	var ev store.Event
	err := s.inTx(ctx, func(q *store.Queries) error {
		var err error
		ev, err = q.GetEventForUpdate(ctx, eventID) // блокировка: места считаются без гонок
		if err != nil {
			return notFound(err)
		}
		if ev.Status != "published" || ev.Day.Before(s.today()) {
			return ErrNotFound
		}
		if !ev.SignupEnabled {
			return &ConflictError{Msg: "Запись на это мероприятие закрыта"}
		}
		if in.WithDog && !ev.DogsAllowed {
			in.WithDog = false
		}
		taken, err := q.EventTaken(ctx, ev.ID)
		if err != nil {
			return err
		}
		left := int(ev.Seats - taken)
		if left <= 0 {
			return &ConflictError{Msg: "Мест больше нет"}
		}
		if in.People > left {
			return &ConflictError{Msg: fmt.Sprintf("Осталось только %d %s", left, Plural(left, "место", "места", "мест"))}
		}
		amount := ev.PriceKop * int64(in.People)
		free = amount == 0
		params := store.CreateSignupParams{EventID: ev.ID, Name: in.Name, Via: in.Via, Contact: contact, People: int32(in.People), WithDog: in.WithDog, AmountKop: amount}
		now := s.now()
		if free {
			params.Status, params.PaidAt = "confirmed", &now
		} else {
			hold := now.Add(SignupHold)
			params.Status, params.HoldUntil = "awaiting_payment", &hold
		}
		su, err = q.CreateSignup(ctx, params)
		if err != nil {
			return err
		}
		c = Checkout{ID: su.ID, Kind: "signup", AmountKop: amount, Status: "awaiting_payment"}
		if free {
			c.Status = "confirmed"
			return nil
		}
		p, err = createPayment(ctx, q, s.pay.Name(), "signup", su.ID, amount, fmt.Sprintf("%s — %d %s, КофеСобакен", ev.Title, in.People, Plural(in.People, "место", "места", "мест")))
		return err
	})
	if err != nil {
		return Checkout{}, err
	}
	if free {
		s.notifySignup(ctx, ev, su)
		return c, nil
	}
	phone := ""
	if in.Via == "phone" {
		phone = contact
	}
	return c, s.startPayment(ctx, p, &c, phone)
}

// ---------- уведомление платёжной системы ----------

type PaymentStatus struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	TargetID    int64     `json:"target_id"`
	AmountKop   int64     `json:"amount_kop"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
}

func toStatus(p store.Payment) PaymentStatus {
	return PaymentStatus{ID: p.ID, Kind: p.Kind, TargetID: p.TargetID, AmountKop: p.AmountKop, Description: p.Description, Status: p.Status}
}

// Payment — статус платежа. Если платёж ещё ждёт оплаты, а платёжная система умеет отвечать о статусе
// (ЮKassa), — спрашиваем её: гость вернулся на сайт раньше уведомления или сервер спал.
func (s *Service) Payment(ctx context.Context, id uuid.UUID) (PaymentStatus, error) {
	p, err := s.q.GetPayment(ctx, id)
	if err != nil {
		return PaymentStatus{}, notFound(err)
	}
	ch, ok := s.pay.(payments.Checker)
	if !ok || p.Status != "pending" || p.ProviderPaymentID == "" {
		return toStatus(p), nil
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	e, err := ch.Check(cctx, p.ProviderPaymentID)
	if err != nil {
		if !errors.Is(err, payments.ErrIgnored) {
			slog.Warn("проверка статуса платежа", "payment_id", p.ID, "err", err)
		}
		return toStatus(p), nil
	}
	if e.PaymentID != p.ID {
		slog.Warn("проверка статуса платежа: чужой платёж", "payment_id", p.ID)
		return toStatus(p), nil
	}
	if err := s.HandlePaymentEvent(ctx, e); err != nil {
		slog.Warn("проверка статуса платежа", "payment_id", p.ID, "err", err)
	}
	if p, err = s.q.GetPayment(ctx, id); err != nil {
		return PaymentStatus{}, notFound(err)
	}
	return toStatus(p), nil
}

// HandlePaymentEvent применяет проверенное уведомление. Повторное уведомление ничего не меняет.
func (s *Service) HandlePaymentEvent(ctx context.Context, e payments.Event) error {
	var after func()
	err := s.inTx(ctx, func(q *store.Queries) error {
		p, err := q.GetPaymentForUpdate(ctx, e.PaymentID)
		if err != nil {
			return notFound(err)
		}
		if e.Type == payments.EventRefunded {
			// Возврат прошёл (с сайта при отмене брони или вручную в кабинете платёжной системы).
			if p.Status != "succeeded" {
				return nil // уже отмечен или платежа не было
			}
			return q.SetPaymentStatus(ctx, store.SetPaymentStatusParams{ID: p.ID, Status: "refunded"})
		}
		if p.Status != "pending" {
			return nil // уже обработано — идемпотентность
		}
		if p.ProviderPaymentID != "" && e.ProviderPaymentID != p.ProviderPaymentID {
			return &ConflictError{Msg: "платёж не совпадает"}
		}
		if e.Type == payments.EventCanceled {
			if err := q.SetPaymentStatus(ctx, store.SetPaymentStatusParams{ID: p.ID, Status: "canceled"}); err != nil {
				return err
			}
			return closeUnpaid(ctx, q, p) // оплаты не будет — бронь / заказ / запись закрываются
		}
		if e.AmountKop != p.AmountKop {
			return &ConflictError{Msg: "сумма платежа не совпадает"}
		}
		if err := q.SetPaymentStatus(ctx, store.SetPaymentStatusParams{ID: p.ID, Status: "succeeded"}); err != nil {
			return err
		}
		switch p.Kind {
		case "booking":
			b, err := q.MarkBookingPaid(ctx, p.TargetID)
			if err != nil {
				return notFound(err)
			}
			after = func() { s.notifyBooking(ctx, b) }
		case "order":
			o, err := q.MarkOrderPaid(ctx, p.TargetID)
			if err != nil {
				return notFound(err)
			}
			items, err := q.ListOrderItems(ctx, []int64{o.ID})
			if err != nil {
				return err
			}
			after = func() { s.notifyOrder(ctx, o, items) }
		case "signup":
			su, err := q.ConfirmSignup(ctx, p.TargetID)
			if err != nil {
				return notFound(err)
			}
			ev, err := q.GetEvent(ctx, su.EventID)
			if err != nil {
				return err
			}
			after = func() { s.notifySignup(ctx, ev, su) }
		}
		return nil
	})
	if err != nil {
		return err
	}
	if after != nil {
		after()
	}
	return nil
}

// TestPay / TestCancel — кнопки страницы тестовой оплаты. Работают только с тестовой заглушкой.
func (s *Service) TestPay(ctx context.Context, id uuid.UUID) (PaymentStatus, error) {
	return s.simulate(ctx, id, payments.EventSucceeded)
}

func (s *Service) TestCancel(ctx context.Context, id uuid.UUID) (PaymentStatus, error) {
	return s.simulate(ctx, id, payments.EventCanceled)
}

func (s *Service) simulate(ctx context.Context, id uuid.UUID, ev string) (PaymentStatus, error) {
	tp, ok := s.pay.(*payments.TestProvider)
	if !ok {
		return PaymentStatus{}, ErrNotFound
	}
	p, err := s.q.GetPayment(ctx, id)
	if err != nil {
		return PaymentStatus{}, notFound(err)
	}
	if p.Status == "pending" {
		if err := tp.Simulate(ctx, ev, p.ID, p.ProviderPaymentID, p.AmountKop); err != nil {
			return PaymentStatus{}, err
		}
	}
	return s.Payment(ctx, id)
}

// refund возвращает деньги через платёжную систему.
func (s *Service) refund(ctx context.Context, q *store.Queries, kind string, target int64) error {
	p, err := q.GetPaymentByTarget(ctx, store.GetPaymentByTargetParams{Kind: kind, TargetID: target})
	if err != nil {
		return notFound(err)
	}
	if p.Status != "succeeded" {
		return nil
	}
	if err := s.pay.Refund(ctx, p.ProviderPaymentID, p.AmountKop); err != nil {
		return err
	}
	return q.SetPaymentStatus(ctx, store.SetPaymentStatusParams{ID: p.ID, Status: "refunded"})
}

func dayTime(d time.Time, clock string) string {
	return strings.TrimSpace(HumanDate(d) + ", " + clock)
}
