// Package config читает настройки сервера из переменных окружения.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"kofesobaken/backend/internal/notify"
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
	ResendAPIKey     string // почта через Resend; пусто — письма только в журнал
	MailFrom         string // отправитель; до подтверждения домена — onboarding@resend.dev
	MailDomainOK     bool   // MAIL_DOMAIN_VERIFIED: у Resend подтверждён домен — письма уходят и гостям (ответы на вопросы)
	TelegramToken    string // бот Telegram; пусто — сообщения только в журнал
	YooKassaShopID   string // ЮKassa: shopId магазина (тестового или боевого)
	YooKassaSecret   string // ЮKassa: секретный ключ (test_… — тестовый магазин, live_… — боевой)
	YooKassaReceipt  bool   // чек 54-ФЗ через ЮKassa (для ИП); самозанятый — false, чек в «Мой налог»
	YooKassaVATCode  int    // ставка НДС в чеке ЮKassa: 1 — без НДС
	KeepAwake        bool   // сервер сам заходит на свой адрес раз в 10 минут (бесплатный Render засыпает через 15)
	Warnings         []string
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
		ResendAPIKey:     os.Getenv("RESEND_API_KEY"),
		MailFrom:         get("MAIL_FROM", notify.DefaultMailFrom),
		MailDomainOK:     get("MAIL_DOMAIN_VERIFIED", "false") == "true",
		TelegramToken:    os.Getenv("TELEGRAM_BOT_TOKEN"),
		YooKassaShopID:   get("YOOKASSA_SHOP_ID", ""),
		YooKassaSecret:   strings.TrimSpace(os.Getenv("YOOKASSA_SECRET_KEY")),
		YooKassaReceipt:  get("YOOKASSA_RECEIPT", "false") == "true",
		// На Render (переменная RENDER=true) включено по умолчанию; KEEP_AWAKE=false — выключить.
		KeepAwake: get("KEEP_AWAKE", get("RENDER", "false")) == "true",
	}
	vat, err := strconv.Atoi(get("YOOKASSA_VAT_CODE", "1"))
	if err != nil || vat < 1 || vat > 12 {
		return c, errors.New("YOOKASSA_VAT_CODE: число от 1 до 12")
	}
	c.YooKassaVATCode = vat
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
	if c.MailDomainOK && (c.ResendAPIKey == "" || strings.HasSuffix(c.MailFrom, "@resend.dev>") || strings.HasSuffix(c.MailFrom, "@resend.dev")) {
		// без своего отправителя Resend доставляет только на почту владельца аккаунта — гостям письма не дойдут
		c.Warnings = append(c.Warnings, "MAIL_DOMAIN_VERIFIED=true, но нет RESEND_API_KEY или MAIL_FROM на своём домене — ответы гостям письмом выключены")
		c.MailDomainOK = false
	}
	if c.MediaStore != "disk" && c.MediaStore != "postgres" {
		return c, errors.New("MEDIA_STORE: disk или postgres")
	}
	switch c.PaymentProvider {
	case "test":
	case "yookassa":
		if c.YooKassaShopID == "" || c.YooKassaSecret == "" {
			// Ключей ещё нет — работаем как раньше, с тестовой страницей оплаты.
			c.Warnings = append(c.Warnings, "PAYMENT_PROVIDER=yookassa, но YOOKASSA_SHOP_ID или YOOKASSA_SECRET_KEY не заданы — включена тестовая страница оплаты")
			c.PaymentProvider = "test"
		}
	default:
		return c, errors.New("PAYMENT_PROVIDER: test или yookassa")
	}
	return c, nil
}
