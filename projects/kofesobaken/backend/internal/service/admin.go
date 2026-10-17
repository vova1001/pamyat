package service

import (
	"context"
	"time"

	"kofesobaken/backend/internal/store"
)

// ---------- брони ----------

type Booking struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Date      string    `json:"date"`
	Time      string    `json:"time"`
	Guests    int32     `json:"guests"`
	WithDog   bool      `json:"with_dog"`
	PrepayKop int64     `json:"prepay_kop"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func toBooking(b store.Booking) Booking {
	return Booking{ID: b.ID, Name: b.Name, Phone: b.Phone, Date: Date(b.Day), Time: Clock(b.At), Guests: b.Guests, WithDog: b.WithDog, PrepayKop: b.PrepayKop, Status: b.Status, CreatedAt: b.CreatedAt}
}

func (s *Service) ListBookings(ctx context.Context) ([]Booking, error) {
	rows, err := s.q.ListBookings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Booking, 0, len(rows))
	for _, b := range rows {
		out = append(out, toBooking(b))
	}
	return out, nil
}

func (s *Service) ConfirmBooking(ctx context.Context, id int64) (Booking, error) {
	b, err := s.q.SetBookingStatus(ctx, store.SetBookingStatusParams{ID: id, Status: "confirmed", FromStatus: []string{"new"}})
	if err != nil {
		return Booking{}, s.statusConflict(ctx, err, id, "Эту бронь уже нельзя подтвердить")
	}
	return toBooking(b), nil
}

func (s *Service) CancelBooking(ctx context.Context, id int64) (Booking, error) {
	var out store.Booking
	err := s.inTx(ctx, func(q *store.Queries) error {
		b, err := q.SetBookingStatus(ctx, store.SetBookingStatusParams{ID: id, Status: "cancelled", FromStatus: []string{"new", "confirmed"}})
		if err != nil {
			return s.statusConflict(ctx, err, id, "Бронь уже отменена")
		}
		out = b
		return s.refund(ctx, q, "booking", id)
	})
	return toBooking(out), err
}

func (s *Service) statusConflict(ctx context.Context, err error, id int64, msg string) error {
	if notFound(err) != ErrNotFound {
		return err
	}
	if _, e := s.q.GetBooking(ctx, id); e != nil {
		return ErrNotFound
	}
	return &ConflictError{Msg: msg}
}

// ---------- заказы ----------

type OrderLine struct {
	Name     string `json:"name"`
	PriceKop int64  `json:"price_kop"`
	Qty      int32  `json:"qty"`
}

type Order struct {
	ID        int64       `json:"id"`
	Mode      string      `json:"mode"`
	Phone     string      `json:"phone"`
	Address   string      `json:"address"`
	TotalKop  int64       `json:"total_kop"`
	Status    string      `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
	Items     []OrderLine `json:"items"`
}

func toOrder(o store.Order, items []store.OrderItem) Order {
	out := Order{ID: o.ID, Mode: o.Mode, Phone: o.Phone, Address: o.Address, TotalKop: o.TotalKop, Status: o.Status, CreatedAt: o.CreatedAt, Items: []OrderLine{}}
	for _, it := range items {
		if it.OrderID == o.ID {
			out.Items = append(out.Items, OrderLine{Name: it.Name, PriceKop: it.PriceKop, Qty: it.Qty})
		}
	}
	return out
}

func (s *Service) ListOrders(ctx context.Context) ([]Order, error) {
	rows, err := s.q.ListOrders(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, o := range rows {
		ids = append(ids, o.ID)
	}
	items, err := s.q.ListOrderItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Order, 0, len(rows))
	for _, o := range rows {
		out = append(out, toOrder(o, items))
	}
	return out, nil
}

func (s *Service) setOrder(ctx context.Context, id int64, status string, refund bool) (Order, error) {
	var out Order
	err := s.inTx(ctx, func(q *store.Queries) error {
		o, err := q.SetOrderStatus(ctx, store.SetOrderStatusParams{ID: id, Status: status})
		if err != nil {
			if notFound(err) == ErrNotFound {
				if _, e := q.GetOrder(ctx, id); e != nil {
					return ErrNotFound
				}
				return &ConflictError{Msg: "Заказ уже закрыт"}
			}
			return err
		}
		items, err := q.ListOrderItems(ctx, []int64{o.ID})
		if err != nil {
			return err
		}
		out = toOrder(o, items)
		if refund {
			return s.refund(ctx, q, "order", id)
		}
		return nil
	})
	return out, err
}

func (s *Service) OrderDone(ctx context.Context, id int64) (Order, error) {
	return s.setOrder(ctx, id, "done", false)
}

func (s *Service) CancelOrder(ctx context.Context, id int64) (Order, error) {
	return s.setOrder(ctx, id, "cancelled", true)
}

// ---------- мероприятия ----------

type PublicEvent struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Date          string `json:"date"`
	Time          string `json:"time"`
	Description   string `json:"description"`
	Photo         string `json:"photo"`
	Seats         int32  `json:"seats"`
	SeatsLeft     int32  `json:"seats_left"`
	PriceKop      int64  `json:"price_kop"`
	DogsAllowed   bool   `json:"dogs_allowed"`
	SignupEnabled bool   `json:"signup_enabled"`
}

type AdminEvent struct {
	PublicEvent
	Status string `json:"status"`
	Taken  int32  `json:"taken"`
	Past   bool   `json:"past"`
}

func publicEvent(e store.Event, taken int32) PublicEvent {
	left := e.Seats - taken
	if left < 0 {
		left = 0
	}
	return PublicEvent{ID: e.ID, Title: e.Title, Kind: e.Kind, Date: Date(e.Day), Time: Clock(e.At), Description: e.Description, Photo: e.Photo,
		Seats: e.Seats, SeatsLeft: left, PriceKop: e.PriceKop, DogsAllowed: e.DogsAllowed, SignupEnabled: e.SignupEnabled}
}

func (s *Service) adminEvent(e store.Event, taken int32) AdminEvent {
	return AdminEvent{PublicEvent: publicEvent(e, taken), Status: e.Status, Taken: taken, Past: e.Day.Before(s.today())}
}

func rowEvent(r store.ListEventsRow) store.Event {
	return store.Event{ID: r.ID, Title: r.Title, Kind: r.Kind, Day: r.Day, At: r.At, Description: r.Description, Photo: r.Photo, Seats: r.Seats,
		PriceKop: r.PriceKop, DogsAllowed: r.DogsAllowed, SignupEnabled: r.SignupEnabled, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (s *Service) ListEvents(ctx context.Context) ([]AdminEvent, error) {
	rows, err := s.q.ListEvents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.adminEvent(rowEvent(r), r.Taken))
	}
	return out, nil
}

type EventInput struct {
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Date          string `json:"date"`
	Time          string `json:"time"`
	Description   string `json:"description"`
	Photo         string `json:"photo"`
	Seats         int    `json:"seats"`
	PriceKop      int64  `json:"price_kop"`
	DogsAllowed   *bool  `json:"dogs_allowed"`
	SignupEnabled *bool  `json:"signup_enabled"`
	Status        string `json:"status"`
}

var eventKinds = map[string]bool{"встреча": true, "мастер-класс": true, "праздник": true, "другое": true}

func (in *EventInput) params() (store.CreateEventParams, error) {
	f := fields{}
	in.Title, in.Description, in.Photo = cleanText(in.Title), cleanText(in.Description), cleanText(in.Photo)
	checkLen(f, "title", in.Title, 1, 80, "Укажите название (до 80 символов)")
	checkLen(f, "description", in.Description, 0, 200, "Не больше 200 символов")
	if !eventKinds[in.Kind] {
		f.add("kind", "Выберите, что это")
	}
	day, ok := ParseDate(in.Date)
	if !ok {
		f.add("date", "Укажите дату")
	}
	at, okT := ParseClock(in.Time)
	if !okT {
		f.add("time", "Укажите время")
	}
	if in.Seats < 1 || in.Seats > 500 {
		f.add("seats", "Мест — от 1 до 500")
	}
	if in.PriceKop < 0 || in.PriceKop > 100_000_00 {
		f.add("price_kop", "Проверьте цену")
	}
	if in.Status != "draft" && in.Status != "published" {
		f.add("status", "Опубликовать или в черновик")
	}
	t := true
	if in.DogsAllowed == nil {
		in.DogsAllowed = &t
	}
	if in.SignupEnabled == nil {
		in.SignupEnabled = &t
	}
	return store.CreateEventParams{Title: in.Title, Kind: in.Kind, Day: day, At: at, Description: in.Description, Photo: in.Photo, Seats: int32(in.Seats),
		PriceKop: in.PriceKop, DogsAllowed: *in.DogsAllowed, SignupEnabled: *in.SignupEnabled, Status: in.Status}, f.err()
}

func (s *Service) CreateEvent(ctx context.Context, in EventInput) (AdminEvent, error) {
	p, err := in.params()
	if err != nil {
		return AdminEvent{}, err
	}
	if p.Day.Before(s.today()) {
		return AdminEvent{}, &ValidationError{Fields: map[string]string{"date": "Эта дата уже прошла"}}
	}
	e, err := s.q.CreateEvent(ctx, p)
	if err != nil {
		return AdminEvent{}, err
	}
	return s.adminEvent(e, 0), nil
}

func (s *Service) UpdateEvent(ctx context.Context, id int64, in EventInput) (AdminEvent, error) {
	p, err := in.params()
	if err != nil {
		return AdminEvent{}, err
	}
	var out AdminEvent
	err = s.inTx(ctx, func(q *store.Queries) error {
		if _, err := q.GetEventForUpdate(ctx, id); err != nil {
			return notFound(err)
		}
		taken, err := q.EventTaken(ctx, id)
		if err != nil {
			return err
		}
		if int32(in.Seats) < taken {
			return &ValidationError{Fields: map[string]string{"seats": "Уже записались " + itoa(int(taken)) + " — мест не может быть меньше"}}
		}
		e, err := q.UpdateEvent(ctx, store.UpdateEventParams{ID: id, Title: p.Title, Kind: p.Kind, Day: p.Day, At: p.At, Description: p.Description, Photo: p.Photo,
			Seats: p.Seats, PriceKop: p.PriceKop, DogsAllowed: p.DogsAllowed, SignupEnabled: p.SignupEnabled, Status: p.Status})
		if err != nil {
			return err
		}
		out = s.adminEvent(e, taken)
		return nil
	})
	return out, err
}

func (s *Service) DeleteEvent(ctx context.Context, id int64) error {
	n, err := s.q.DeleteEvent(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type Signup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Via       string    `json:"via"`
	Contact   string    `json:"contact"`
	People    int32     `json:"people"`
	WithDog   bool      `json:"with_dog"`
	AmountKop int64     `json:"amount_kop"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) ListSignups(ctx context.Context, eventID int64) ([]Signup, error) {
	if _, err := s.q.GetEvent(ctx, eventID); err != nil {
		return nil, notFound(err)
	}
	rows, err := s.q.ListSignups(ctx, eventID)
	if err != nil {
		return nil, err
	}
	out := make([]Signup, 0, len(rows))
	for _, r := range rows {
		out = append(out, Signup{ID: r.ID, Name: r.Name, Via: r.Via, Contact: r.Contact, People: r.People, WithDog: r.WithDog, AmountKop: r.AmountKop, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// ---------- страница сайта ----------

type SiteOrder struct {
	DeliveryMinKop   int64 `json:"delivery_min_kop"`
	DeliveryRadiusKm int   `json:"delivery_radius_km"`
	BookingPrepayKop int64 `json:"booking_prepay_kop"`
}

type Site struct {
	Content Content          `json:"content"`
	Menu    []store.MenuItem `json:"menu"`
	Events  []PublicEvent    `json:"events"`
	Order   SiteOrder        `json:"order"`
}

func (s *Service) Site(ctx context.Context) (Site, error) {
	c, err := s.Content(ctx)
	if err != nil {
		return Site{}, err
	}
	menu, err := s.q.ListVisibleMenuItems(ctx)
	if err != nil {
		return Site{}, err
	}
	rows, err := s.q.ListPublishedEventsFrom(ctx, s.today())
	if err != nil {
		return Site{}, err
	}
	evs := make([]PublicEvent, 0, len(rows))
	for _, r := range rows {
		e := store.Event{ID: r.ID, Title: r.Title, Kind: r.Kind, Day: r.Day, At: r.At, Description: r.Description, Photo: r.Photo, Seats: r.Seats,
			PriceKop: r.PriceKop, DogsAllowed: r.DogsAllowed, SignupEnabled: r.SignupEnabled, Status: r.Status}
		evs = append(evs, publicEvent(e, r.Taken))
	}
	o, err := s.orderSettings(ctx)
	if err != nil {
		return Site{}, err
	}
	return Site{Content: c, Menu: menu, Events: evs, Order: SiteOrder{DeliveryMinKop: o.DeliveryMinKop, DeliveryRadiusKm: o.DeliveryRadiusKm, BookingPrepayKop: BookingPrepayKop}}, nil
}
