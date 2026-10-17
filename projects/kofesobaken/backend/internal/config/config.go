// Package config читает настройки сервера из переменных окружения.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	PublicURL        string // адрес сайта для гостя (ссылки оплаты и возврата)
	WebhookURL       string // куда тестовая заглушка оплаты шлёт уведомление
	AdminEmail       string
	AdminPassword    string
	PaymentProvider  string
	WebhookSecret    string
	UploadDir        string
	MediaStore       string // где фото: disk (том uploads) или postgres (хостинг без диска)
	AdminLinkOnStart bool   // новая ссылка входа администратора при каждом старте (забыл пароль, а консоли нет)
	AdminPassTemp    bool   // ADMIN_PASSWORD временный: при первом входе — обязательная смена
	Location         *time.Location
	CookieSecure     bool
	SeedDemo         bool
}

func get(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:    get("HTTP_ADDR", ":8080"),
		DatabaseURL: get("DATABASE_URL", ""),
		// На Render адрес сервиса приходит сам (RENDER_EXTERNAL_URL).
		PublicURL:        strings.TrimRight(get("PUBLIC_URL", get("RENDER_EXTERNAL_URL", "http://localhost:8000")), "/"),
		WebhookURL:       get("PAYMENT_WEBHOOK_URL", "http://localhost:8080/api/payments/webhook"),
		AdminEmail:       strings.ToLower(get("ADMIN_EMAIL", "")),
		AdminPassword:    os.Getenv("ADMIN_PASSWORD"),
		PaymentProvider:  get("PAYMENT_PROVIDER", "test"),
		WebhookSecret:    os.Getenv("PAYMENT_WEBHOOK_SECRET"),
		UploadDir:        get("UPLOAD_DIR", "/data/uploads"),
		MediaStore:       get("MEDIA_STORE", "disk"),
		AdminLinkOnStart: get("ADMIN_LINK_ON_START", "false") == "true",
		AdminPassTemp:    get("ADMIN_PASSWORD_TEMPORARY", "false") == "true",
		CookieSecure:     get("COOKIE_SECURE", "false") == "true",
		SeedDemo:         get("SEED_DEMO", "false") == "true",
	}
	loc, err := time.LoadLocation(get("CAFE_TZ", "Europe/Moscow"))
	if err != nil {
		return c, err
	}
	c.Location = loc
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL не задан")
	}
	if c.WebhookSecret == "" {
		// Для тестовой заглушки секрет нужен только внутри сервера — создаём случайный.
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		c.WebhookSecret = hex.EncodeToString(b)
	}
	if c.MediaStore != "disk" && c.MediaStore != "postgres" {
		return c, errors.New("MEDIA_STORE: disk или postgres")
	}
	if c.PaymentProvider != "test" {
		return c, errors.New("PAYMENT_PROVIDER: пока доступна только тестовая заглушка (test)")
	}
	return c, nil
}
