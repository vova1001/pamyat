package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"

	"kofesobaken/backend/internal/store"
)

const (
	keyContent = "content"
	keyOrder   = "order"
	keyNotify  = "notify"
)

type WhyCard struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Photo string `json:"photo"`
}

type Dog struct {
	Name  string `json:"name"`
	About string `json:"about"`
	Photo string `json:"photo"`
}

// Content — тексты и фото сайта, которые владелец меняет в кабинете.
type Content struct {
	HeroChip  string    `json:"hero_chip"`
	HeroLead  string    `json:"hero_lead"`
	HeroPhoto string    `json:"hero_photo"`
	Address   string    `json:"address"`
	Hours     string    `json:"hours"`
	Why       []WhyCard `json:"why"`
	Dogs      []Dog     `json:"dogs"`
	HowToGet  string    `json:"how_to_get"`
}

// OrderSettings — условия заказа. Решение заказчика: доставка — от 1000 ₽ и бесплатно, радиус 10 км.
type OrderSettings struct {
	DeliveryMinKop   int64 `json:"delivery_min_kop"`
	DeliveryRadiusKm int   `json:"delivery_radius_km"`
}

type NotifySettings struct {
	EmailEnabled      bool   `json:"email_enabled"`
	Email             string `json:"email"`
	TelegramConnected bool   `json:"telegram_connected"`
	TelegramChat      string `json:"telegram_chat"`              // кого показать в кабинете: @имя
	TelegramChatID    string `json:"telegram_chat_id,omitempty"` // куда слать (chat_id из Telegram); пусто — заглушка
	OnBooking         bool   `json:"on_booking"`
	OnSignup          bool   `json:"on_signup"`
	OnFull            bool   `json:"on_full"`
	OnOrder           bool   `json:"on_order"`
}

type Settings struct {
	DeliveryMinKop   int64          `json:"delivery_min_kop"`
	DeliveryRadiusKm int            `json:"delivery_radius_km"`
	BookingPrepayKop int64          `json:"booking_prepay_kop"`
	Notify           NotifySettings `json:"notify"`
}

// DefaultContent — тексты и фото из согласованного эскиза.
func DefaultContent() Content {
	return Content{
		HeroChip:  "Кофейня с собаками · Ростов-на-Дону",
		HeroLead:  "Кофе, который пьют **в компании маленьких собак.** Приходите сами или со своим питомцем — здесь рады всем.",
		HeroPhoto: "/img/hero-pomeranian-coffee.jpg",
		Address:   "ул. Сарьяна, 34",
		Hours:     "ежедневно 9:00–21:00",
		Why: []WhyCard{
			{Title: "Собаки в зале", Text: "По залу бегают маленькие собаки: можно погладить, поиграть или просто смотреть, как они спят у окна. Собака спит или ушла в домик — значит, отдыхает, не будим. Угощать их можно только лакомством от бариста.", Photo: "/img/poodle-sweater.jpg"},
			{Title: "Можно со своим питомцем", Text: "Берите собаку с собой — ей здесь рады так же, как вам. В зале держите её на поводке и рядом со столиком, на стулья и столы не пускаем. Миску с водой принесём сами. Если собака волнуется среди других — выберите столик у входа, там спокойнее.", Photo: "/img/guest-with-dog.jpg"},
			{Title: "Уютно, как дома", Text: "Тёплый свет, мягкие места и кофе, за которым хочется задержаться подольше. Место, куда приходят выдохнуть.", Photo: "/img/cozy-table.jpg"},
		},
		Dogs: []Dog{
			{Name: "Пончик", About: "шпиц — первым бежит встречать у двери и знает, у кого в кармане лакомство.", Photo: "/img/dog-green.jpg"},
			{Name: "Ириска", About: "той-пудель — любит колени гостей и может проспать на них весь капучино.", Photo: "/img/dog-yellow.jpg"},
			{Name: "Бублик", About: "корги — главный по мячику. Бросите один раз — придётся играть до закрытия.", Photo: "/img/dog-red.jpg"},
			{Name: "Кнопка", About: "чихуахуа — сначала смотрит издалека, а потом выбирает самый тёплый плед.", Photo: "/img/dog-mint.jpg"},
		},
		HowToGet: "Вход с улицы Сарьяна. Ближайшие остановки — «29-я линия» и «35-я линия». Рядом «Пятёрочка» и «Фасоль». Машину можно оставить вдоль улицы.",
	}
}

func DefaultOrderSettings() OrderSettings {
	return OrderSettings{DeliveryMinKop: 1000_00, DeliveryRadiusKm: 10}
}

func DefaultNotify() NotifySettings {
	return NotifySettings{
		EmailEnabled: true, Email: "vovaoleshko05@mail.ru",
		TelegramConnected: false, TelegramChat: "@ludi_bydte_dobree",
		OnBooking: true, OnSignup: true, OnFull: true, OnOrder: true,
	}
}

func getJSON[T any](ctx context.Context, q *store.Queries, key string, def T) (T, error) {
	raw, err := q.GetSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	v := def
	if err := json.Unmarshal(raw, &v); err != nil {
		return def, err
	}
	return v, nil
}

func putJSON(ctx context.Context, q *store.Queries, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return q.PutSetting(ctx, store.PutSettingParams{Key: key, Value: raw})
}

func (s *Service) Content(ctx context.Context) (Content, error) {
	return getJSON(ctx, s.q, keyContent, DefaultContent())
}

func (s *Service) orderSettings(ctx context.Context) (OrderSettings, error) {
	return getJSON(ctx, s.q, keyOrder, DefaultOrderSettings())
}

func (s *Service) notifySettings(ctx context.Context) (NotifySettings, error) {
	n, err := getJSON(ctx, s.q, keyNotify, DefaultNotify())
	// Подключено заглушкой (без бота), а теперь бот есть — чата нет: нужно подключить по-настоящему.
	if _, bot := s.telegram(); bot && n.TelegramChatID == "" {
		n.TelegramConnected = false
	}
	return n, err
}

func (s *Service) PutContent(ctx context.Context, c Content) (Content, error) {
	f := fields{}
	c.HeroChip, c.HeroLead, c.Address, c.Hours, c.HowToGet = cleanText(c.HeroChip), cleanText(c.HeroLead), cleanText(c.Address), cleanText(c.Hours), cleanText(c.HowToGet)
	checkLen(f, "hero_chip", c.HeroChip, 1, 80, "Подпись — от 1 до 80 символов")
	checkLen(f, "hero_lead", c.HeroLead, 1, 160, "Текст под названием — от 1 до 160 символов")
	checkLen(f, "address", c.Address, 1, 80, "Укажите адрес")
	checkLen(f, "hours", c.Hours, 1, 80, "Укажите часы работы")
	checkLen(f, "how_to_get", c.HowToGet, 0, 300, "Не больше 300 символов")
	if c.HeroPhoto == "" {
		f.add("hero_photo", "Нужно главное фото")
	}
	if len(c.Why) != 3 {
		f.add("why", "Нужно ровно три карточки")
	}
	for i := range c.Why {
		c.Why[i].Title, c.Why[i].Text = cleanText(c.Why[i].Title), cleanText(c.Why[i].Text)
		checkLen(f, "why", c.Why[i].Title, 1, 60, "У каждой карточки нужен заголовок до 60 символов")
		checkLen(f, "why", c.Why[i].Text, 1, 300, "У каждой карточки нужен текст до 300 символов")
	}
	if len(c.Dogs) > 12 {
		f.add("dogs", "Не больше 12 собак")
	}
	dogs := c.Dogs[:0]
	for _, d := range c.Dogs {
		d.Name, d.About = cleanText(d.Name), cleanText(d.About)
		checkLen(f, "dogs", d.Name, 0, 40, "Имя собаки — до 40 символов")
		checkLen(f, "dogs", d.About, 0, 80, "О собаке — до 80 символов")
		if d.Photo == "" && d.Name == "" && d.About == "" {
			continue // пустая карточка не сохраняется
		}
		dogs = append(dogs, d)
	}
	c.Dogs = dogs
	if err := f.err(); err != nil {
		return c, err
	}
	if c.Dogs == nil {
		c.Dogs = []Dog{}
	}
	return c, putJSON(ctx, s.q, keyContent, c)
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	o, err := s.orderSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	n, err := s.notifySettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return Settings{DeliveryMinKop: o.DeliveryMinKop, DeliveryRadiusKm: o.DeliveryRadiusKm, BookingPrepayKop: BookingPrepayKop, Notify: n}, nil
}

func (s *Service) PutSettings(ctx context.Context, in Settings) (Settings, error) {
	f := fields{}
	if in.DeliveryMinKop < 0 || in.DeliveryMinKop > 100_000_00 {
		f.add("delivery_min_kop", "Сумма — от 0 до 100 000 ₽")
	}
	if in.DeliveryRadiusKm < 1 || in.DeliveryRadiusKm > 50 {
		f.add("delivery_radius_km", "Радиус — от 1 до 50 км")
	}
	n := in.Notify
	n.Email = strings.TrimSpace(n.Email)
	if n.Email != "" {
		if a, err := mail.ParseAddress(n.Email); err != nil || a.Address != n.Email {
			f.add("email", "Проверьте адрес почты")
		}
	}
	if n.EmailEnabled && n.Email == "" {
		f.add("email", "Укажите почту или выключите письма")
	}
	if err := f.err(); err != nil {
		return in, err
	}
	// Подключение Telegram меняется только через connect/disconnect.
	cur, err := s.notifySettings(ctx)
	if err != nil {
		return in, err
	}
	n.TelegramConnected, n.TelegramChat, n.TelegramChatID = cur.TelegramConnected, cur.TelegramChat, cur.TelegramChatID
	err = s.inTx(ctx, func(q *store.Queries) error {
		if err := putJSON(ctx, q, keyOrder, OrderSettings{DeliveryMinKop: in.DeliveryMinKop, DeliveryRadiusKm: in.DeliveryRadiusKm}); err != nil {
			return err
		}
		return putJSON(ctx, q, keyNotify, n)
	})
	if err != nil {
		return in, err
	}
	return s.Settings(ctx)
}
