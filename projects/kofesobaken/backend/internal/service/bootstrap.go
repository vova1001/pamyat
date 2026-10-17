package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"kofesobaken/backend/internal/store"
)

// Bootstrap готовит пустую базу: владелец кабинета, тексты и настройки по умолчанию, стартовое меню.
// Повторный запуск ничего не перезаписывает, кроме пароля владельца из окружения.
func (s *Service) Bootstrap(ctx context.Context, adminEmail, adminPassword string, seedDemo bool) error {
	if adminEmail != "" && adminPassword != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := s.q.UpsertAdmin(ctx, store.UpsertAdminParams{Email: strings.ToLower(adminEmail), PasswordHash: string(h)}); err != nil {
			return err
		}
	} else if adminEmail != "" {
		// Пароль не задан: при первом запуске создаём случайный и один раз пишем в лог.
		if _, err := s.q.GetAdminByEmail(ctx, strings.ToLower(adminEmail)); err != nil {
			b := make([]byte, 9)
			_, _ = rand.Read(b)
			pass := base64.RawURLEncoding.EncodeToString(b)
			h, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			if _, err := s.q.UpsertAdmin(ctx, store.UpsertAdminParams{Email: strings.ToLower(adminEmail), PasswordHash: string(h)}); err != nil {
				return err
			}
			slog.Warn("ADMIN_PASSWORD не задан — создан временный пароль кабинета; задайте свой в .env", "email", adminEmail, "password", pass)
		}
	} else {
		slog.Warn("ADMIN_EMAIL не задан — вход в кабинет не настроен")
	}
	for key, v := range map[string]any{keyContent: DefaultContent(), keyOrder: DefaultOrderSettings(), keyNotify: DefaultNotify()} {
		raw, _ := json.Marshal(v)
		if err := s.q.InsertSettingIfMissing(ctx, store.InsertSettingIfMissingParams{Key: key, Value: raw}); err != nil {
			return err
		}
	}
	n, err := s.q.CountMenuItems(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		for _, m := range defaultMenu() {
			if _, err := s.q.CreateMenuItem(ctx, m); err != nil {
				return err
			}
		}
	}
	if seedDemo {
		return s.seedDemoEvents(ctx)
	}
	return nil
}

// defaultMenu — меню из согласованного эскиза; цены примерные (подобраны по рынку), заказчик подтвердит.
func defaultMenu() []store.CreateMenuItemParams {
	return []store.CreateMenuItemParams{
		{Name: "Эспрессо", Photo: "/img/cup-wood.jpg", Description: "Короткий и плотный — для бодрого старта", PriceKop: 300_00, Category: "black", Visible: true},
		{Name: "Американо", Description: "Эспрессо и горячая вода", PriceKop: 300_00, Category: "black", Visible: true},
		{Name: "Капучино", Photo: "/img/latte-pour.jpg", Description: "Эспрессо и бархатная молочная пена", PriceKop: 380_00, Category: "milk", Visible: true},
		{Name: "Латте", Photo: "/img/latte-glass.jpg", Description: "Мягкий, много молока", PriceKop: 400_00, Category: "milk", Visible: true},
		{Name: "Флэт уайт", Description: "Двойной эспрессо и немного молока — крепче латте", PriceKop: 420_00, Category: "milk", Visible: true},
		{Name: "Раф", Description: "Эспрессо, сливки и ванильный сахар", PriceKop: 480_00, Category: "milk", Visible: true},
		{Name: "Какао", Description: "Тёплое и шоколадное", PriceKop: 380_00, Category: "other", Visible: true},
		{Name: "Чай", Description: "Чёрный, зелёный или травяной", PriceKop: 300_00, Category: "other", Visible: true},
		{Name: "Авторский напиток", Photo: "/img/iced-latte.jpg", Description: "[УТОЧНИТЬ: название и состав]", PriceKop: 550_00, Category: "milk", Visible: true, Signature: true},
	}
}

// seedDemoEvents — примеры мероприятий из эскиза для показа (только если мероприятий ещё нет).
func (s *Service) seedDemoEvents(ctx context.Context) error {
	evs, err := s.q.ListEvents(ctx)
	if err != nil || len(evs) > 0 {
		return err
	}
	t := s.today()
	demo := []store.CreateEventParams{
		{Title: "Встреча собаководов", Kind: "встреча", Day: t.AddDate(0, 0, 7), Description: "Знакомимся, болтаем о питомцах за кофе, собаки играют в зале. [УТОЧНИТЬ: программа и цена]", Photo: "/img/guest-with-dog.jpg", Seats: 15, PriceKop: 0, DogsAllowed: true, SignupEnabled: true, Status: "published"},
		{Title: "Латте-арт: рисуем на пенке", Kind: "мастер-класс", Day: t.AddDate(0, 0, 12), Description: "Бариста покажет, как нарисовать сердце и мордочку собаки. [УТОЧНИТЬ: цена]", Photo: "/img/latte-glass.jpg", Seats: 10, PriceKop: 700_00, DogsAllowed: false, SignupEnabled: true, Status: "published"},
		{Title: "День рождения собаки", Kind: "праздник", Day: t.AddDate(0, 0, 15), Description: "Торт для собак, колпаки и общее фото. [УТОЧНИТЬ: чей праздник, условия]", Photo: "/img/dog-red.jpg", Seats: 6, PriceKop: 500_00, DogsAllowed: true, SignupEnabled: true, Status: "published"},
	}
	times := []string{"12:00", "19:00", "15:00"}
	for i, e := range demo {
		e.At, _ = ParseClock(times[i])
		if _, err := s.q.CreateEvent(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
