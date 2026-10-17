package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"kofesobaken/backend/internal/service"
	"kofesobaken/backend/migrations"
)

// Тексты по умолчанию: без заглушек [УТОЧНИТЬ] и проходят проверки кабинета.
func TestDefaultTextsReady(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.Bootstrap(ctx, "owner@example.com", "", true); err != nil {
		t.Fatal(err)
	}
	c, _ := e.svc.Content(ctx)
	menu, _ := e.svc.ListMenu(ctx)
	evs, _ := e.svc.ListEvents(ctx)
	raw, _ := json.Marshal([]any{c, menu, evs})
	if strings.Contains(string(raw), "УТОЧНИТЬ") {
		t.Fatalf("остались заглушки: %s", raw)
	}
	if _, err := e.svc.PutContent(ctx, service.DefaultContent()); err != nil {
		t.Fatalf("тексты по умолчанию не проходят проверку кабинета: %v", err)
	}
	for _, d := range c.Dogs {
		if d.Name == "" || d.About == "" {
			t.Fatalf("собака без имени или описания: %+v", d)
		}
	}
}

// Миграция 00004 меняет только старые заглушки; правки владельца не трогает; обратима.
func TestTextsMigration(t *testing.T) {
	if testPool == nil {
		t.Skip("нужна TEST_DATABASE_URL")
	}
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(testPool)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, ".", 3); err != nil {
		t.Fatal(err)
	}
	old := `{"hero_chip":"x","why":[
	  {"title":"Собаки в зале","text":"По залу бегают маленькие собаки: можно погладить, поиграть или просто смотреть, как они спят у окна. [УТОЧНИТЬ: сколько собак и какие]","photo":"/img/poodle-sweater.jpg"},
	  {"title":"Можно со своим питомцем","text":"Своё правило владельца","photo":"/img/guest-with-dog.jpg"},
	  {"title":"Уютно","text":"т","photo":"/img/cozy-table.jpg"}],
	 "dogs":[{"name":"","about":"","photo":"/img/dog-green.jpg"},{"name":"","about":"","photo":"/img/dog-yellow.jpg"},{"name":"","about":"","photo":"/img/dog-red.jpg"},{"name":"","about":"","photo":"/img/dog-mint.jpg"}],
	 "how_to_get":""}`
	if _, err := testPool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('content', $1)`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO menu_items (name, description, price_kop, category) VALUES
	  ('Авторский напиток', '[УТОЧНИТЬ: название и состав]', 55000, 'milk'), ('Раф', 'Свой текст', 48000, 'milk')`); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	svc := service.New(testPool, nil, nil, msk)
	c, _ := svc.Content(ctx)
	def := service.DefaultContent()
	if c.Why[0].Text != def.Why[0].Text || c.Why[1].Text != "Своё правило владельца" || c.HowToGet != def.HowToGet || c.Dogs[0].Name != "Пончик" || c.HeroChip != "x" {
		t.Fatalf("миграция текстов: %+v", c)
	}
	menu, _ := svc.ListMenu(ctx)
	if menu[0].Name != "Лапа на льду" || menu[1].Description != "Свой текст" {
		t.Fatalf("миграция меню: %+v", menu)
	}
	if err := goose.Down(db, "."); err != nil {
		t.Fatal(err)
	}
	c, _ = svc.Content(ctx)
	if !strings.Contains(c.Why[0].Text, "УТОЧНИТЬ") || c.HowToGet != "" || c.Dogs[0].Name != "" {
		t.Fatalf("обратная миграция: %+v", c)
	}
}

// Данные гостей старше года удаляются, свежие и платежи остаются.
func TestPurgeOld(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO bookings (name, phone, day, at, guests, prepay_kop, status, created_at) VALUES
		  ('Старая', '+7', now() - interval '400 days', '12:00', 2, 0, 'new', now() - interval '400 days'),
		  ('Новая', '+7', now() + interval '1 day', '12:00', 2, 0, 'new', now())`,
		`INSERT INTO orders (mode, phone, total_kop, status, created_at) VALUES
		  ('pickup', '+7', 100, 'done', now() - interval '400 days'), ('pickup', '+7', 100, 'new', now())`,
		`INSERT INTO notifications (channel, kind, recipient, title, body, status, created_at) VALUES
		  ('email', 'order', 'a@b.c', 't', 'b', 'sent', now() - interval '400 days')`,
		`INSERT INTO payments (kind, target_id, amount_kop, description, provider, status, created_at) VALUES
		  ('order', 1, 100, 'Заказ', 'test', 'succeeded', now() - interval '400 days')`,
	} {
		if _, err := testPool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	n, err := e.svc.PurgeOld(ctx)
	if err != nil || n != 3 {
		t.Fatalf("ждали удалить 3 записи, удалено %d: %v", n, err)
	}
	var left, pays int
	_ = testPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM bookings) + (SELECT count(*) FROM orders)`).Scan(&left)
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM payments`).Scan(&pays)
	if left != 2 || pays != 1 {
		t.Fatalf("свежие брони/заказы и платежи должны остаться: %d %d", left, pays)
	}
}
