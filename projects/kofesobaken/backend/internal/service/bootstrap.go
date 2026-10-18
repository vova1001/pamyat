package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"kofesobaken/backend/internal/store"
)

// Bootstrap готовит пустую базу: администратор кабинета, тексты и настройки по умолчанию, стартовое меню.
// Повторный запуск ничего не перезаписывает. ADMIN_PASSWORD (если задан) — только первый пароль
// администратора: свой пароль, сменённый в кабинете, при перезапуске не сбрасывается.
// Без ADMIN_PASSWORD администратор задаёт пароль сам по одноразовой ссылке (см. PendingSetup).
// SetAdminPasswordTemporary(true) — пароль из окружения временный: при первом входе — обязательная смена.
func (s *Service) Bootstrap(ctx context.Context, adminEmail, adminPassword string, seedDemo bool) error {
	if email := normEmail(adminEmail); email != "" {
		if err := s.ensureAdmin(ctx, email, adminPassword); err != nil {
			return err
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

// ensureAdmin — администратор из окружения. Если эту почту он уже сменил в «Профиле», второй вход
// на старую почту не создаётся (иначе ADMIN_PASSWORD снова открыл бы кабинет по старой почте).
func (s *Service) ensureAdmin(ctx context.Context, email, password string) error {
	a, err := s.q.GetAdminByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		b, ferr := s.q.GetAdminByFormerEmail(ctx, email)
		if ferr == nil {
			slog.Info("ADMIN_EMAIL — прежняя почта администратора, сменённая в кабинете; вход — по нынешней", "email", b.Email)
			return nil
		}
		if !errors.Is(ferr, pgx.ErrNoRows) {
			return ferr
		}
	} else if err != nil {
		return err
	}
	hash := ""
	if password != "" && (err != nil || a.PasswordHash == "") {
		if hash, err = hashPassword(password); err != nil {
			return err
		}
	}
	_, err = s.q.EnsureAdmin(ctx, store.EnsureAdminParams{Email: email, PasswordHash: hash, MustChangePassword: s.adminPassTemp})
	return err
}

// PendingSetup выдаёт одноразовую ссылку первого входа, если пароль администратора ещё не задан и
// действующей ссылки нет. Выданная ссылка живёт до срока или до использования: перезапуск (выкладка)
// её не перевыпускает, иначе ссылка, уже отправленная заказчику, перестанет работать.
// Новая ссылка — только явно: команда admin-link или ADMIN_LINK_ON_START (IssueSetupLink).
// Пустая строка — новая ссылка не нужна.
func (s *Service) PendingSetup(ctx context.Context, adminEmail string) (string, error) {
	a, err := s.adminByLogin(ctx, adminEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil || a.PasswordHash != "" {
		return "", err
	}
	if a.SetupTokenHash.Valid && a.SetupExpiresAt != nil && a.SetupExpiresAt.After(s.now()) {
		slog.Info("ссылка входа администратора уже выдана и ещё действует — новая не нужна", "email", a.Email, "until", a.SetupExpiresAt.In(s.loc).Format("02.01.2006 15:04"))
		return "", nil
	}
	return s.IssueSetupLink(ctx, a.Email)
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
		{Name: "Лапа на льду", Photo: "/img/iced-latte.jpg", Description: "Холодный латте: двойной эспрессо, молоко, солёная карамель и много льда.", PriceKop: 550_00, Category: "milk", Visible: true, Signature: true},
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
		{Title: "Встреча собаководов", Kind: "встреча", Day: t.AddDate(0, 0, 7), Description: "Знакомимся, болтаем о питомцах за кофе, собаки играют в зале. В программе — знакомство, свободная игра собак и обмен советами. Вход свободный, кофе — по меню. Можно со своей собакой на поводке.", Photo: "/img/guest-with-dog.jpg", Seats: 15, PriceKop: 0, DogsAllowed: true, SignupEnabled: true, Status: "published"},
		{Title: "Латте-арт: рисуем на пенке", Kind: "мастер-класс", Day: t.AddDate(0, 0, 12), Description: "Бариста покажет, как нарисовать сердце и мордочку собаки. Полтора часа практики: взбиваем молоко, рисуем и пьём свой латте. Опыт не нужен. Участие — 700 ₽.", Photo: "/img/latte-glass.jpg", Seats: 10, PriceKop: 700_00, DogsAllowed: false, SignupEnabled: true, Status: "published"},
		{Title: "День рождения собаки", Kind: "праздник", Day: t.AddDate(0, 0, 15), Description: "Отмечаем дни рождения собак, у которых праздник в этом месяце. Торт для собак, колпаки и общее фото. Участие — 500 ₽ за собаку с хозяином.", Photo: "/img/dog-red.jpg", Seats: 6, PriceKop: 500_00, DogsAllowed: true, SignupEnabled: true, Status: "published"},
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
