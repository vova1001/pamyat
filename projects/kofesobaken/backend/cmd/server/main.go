// Сервер API КофеСобакен: настройки из окружения, миграции, подготовка БД, HTTP.
//
//	server             — запуск
//	server admin-link  — новая одноразовая ссылка входа администратора (забыл пароль): пароль задаётся заново
//	ADMIN_LINK_ON_START=true — то же при каждом старте (хостинг без консоли, например бесплатный Render)
//	Без этого ссылка выдаётся, только пока пароль не задан и прежняя ссылка истекла (перезапуск её не меняет).
//	RESEND_API_KEY, MAIL_FROM, TELEGRAM_BOT_TOKEN — настоящие письма и сообщения; без них — в журнал.
//	MAIL_DOMAIN_VERIFIED=true — домен MAIL_FROM подтверждён в Resend: ответы на вопросы уходят гостям письмом.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // часовой пояс кофейни без системных файлов

	"github.com/jackc/pgx/v5/pgxpool"

	"kofesobaken/backend/internal/config"
	"kofesobaken/backend/internal/httpapi"
	"kofesobaken/backend/internal/media"
	"kofesobaken/backend/internal/notify"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
	"kofesobaken/backend/internal/store"
	"kofesobaken/backend/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("сервер остановлен", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrations.Up(pool); err != nil {
		return err
	}

	for _, w := range cfg.Warnings {
		slog.Warn(w)
	}
	var pay payments.Provider = payments.NewTestProvider(cfg.PublicURL, cfg.WebhookURL, cfg.WebhookSecret)
	if cfg.PaymentProvider == "yookassa" {
		yk := payments.NewYooKassa(cfg.YooKassaShopID, cfg.YooKassaSecret)
		yk.Receipt, yk.VATCode = cfg.YooKassaReceipt, cfg.YooKassaVATCode
		pay = yk
		slog.Info("оплата: ЮKassa", "test_shop", yk.TestMode(), "receipt", yk.Receipt)
	}
	svc := service.New(pool, pay, channels(cfg), cfg.Location)
	svc.SetPublicURL(cfg.PublicURL)
	svc.SetAdminPasswordTemporary(cfg.AdminPassTemp)
	svc.SetMailDomainVerified(cfg.MailDomainOK)
	if err := svc.Bootstrap(ctx, cfg.AdminEmail, cfg.AdminPassword, cfg.SeedDemo); err != nil {
		return err
	}
	// Ссылка входа администратора в лог (одноразовая, 72 часа; в базе — только хэш):
	// команда admin-link или ADMIN_LINK_ON_START — всегда новая; иначе — только пока пароль не задан.
	cmdLink := len(os.Args) > 1 && os.Args[1] == "admin-link"
	var code string
	if cmdLink || cfg.AdminLinkOnStart {
		code, err = svc.IssueSetupLink(ctx, cfg.AdminEmail)
	} else {
		code, err = svc.PendingSetup(ctx, cfg.AdminEmail)
	}
	if err != nil {
		return err
	}
	if code != "" {
		logSetupLink(cfg, code)
		svc.MailSetupLink(ctx, cfg.AdminEmail, setupURL(cfg, code))
	}
	if cmdLink {
		return nil
	}

	go svc.RunRetention(ctx)    // данные гостей храним 1 год (политика конфиденциальности)
	go svc.RunPaymentSweep(ctx) // статус платежей, если уведомление платёжной системы не пришло
	go svc.RunTelegram(ctx)     // сообщения боту: подключение владелицы, вопросы гостей, её ответы реплаем
	if cfg.KeepAwake {
		go keepAwake(ctx, cfg.PublicURL+"/healthz")
	}

	var photos media.Store = media.Disk{Dir: cfg.UploadDir}
	if cfg.MediaStore == "postgres" {
		photos = media.Postgres{Q: store.New(pool)}
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.New(svc, pay, httpapi.Options{Media: photos, CookieSecure: cfg.CookieSecure}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	slog.Info("сервер запущен", "addr", cfg.HTTPAddr, "payments", pay.Name(), "photos", cfg.MediaStore,
		"mail", cfg.ResendAPIKey != "", "mail_guests", cfg.MailDomainOK, "telegram", cfg.TelegramToken != "")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// keepAwake заходит на публичный адрес сайта раз в 10 минут: бесплатный Render усыпляет сервис
// через 15 минут без посетителей, а спящий сервер не проверяет платежи.
func keepAwake(ctx context.Context, url string) {
	cl := &http.Client{Timeout: 30 * time.Second}
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	slog.Info("сайт не засыпает: заход на свой адрес раз в 10 минут", "url", url)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			slog.Warn("пинг сайта", "err", err)
			return
		}
		if resp, err := cl.Do(req); err != nil {
			slog.Warn("пинг сайта", "err", err)
		} else {
			resp.Body.Close()
		}
	}
}

// logSetupLink — ссылка первого входа: по ней администратор сам задаёт пароль. В базе — только хэш кода.
func logSetupLink(cfg config.Config, code string) {
	slog.Warn("ссылка входа администратора: откройте её и придумайте пароль (одноразовая, 72 часа)",
		"email", cfg.AdminEmail, "link", setupURL(cfg, code))
}

func setupURL(cfg config.Config, code string) string { return cfg.PublicURL + "/admin#setup/" + code }

// channels — настоящие адаптеры, если заданы ключи; иначе заглушки (сообщения в журнал), как раньше.
func channels(cfg config.Config) []notify.Channel {
	var mail notify.Channel = notify.LogChannel{Channel_: "email"}
	if cfg.ResendAPIKey != "" {
		mail = notify.Resend{APIKey: cfg.ResendAPIKey, From: cfg.MailFrom}
	}
	var tg notify.Channel = notify.LogChannel{Channel_: "telegram"}
	if cfg.TelegramToken != "" {
		tg = notify.Telegram{Token: cfg.TelegramToken, PublicURL: cfg.PublicURL}
	}
	return []notify.Channel{mail, tg}
}

// connect ждёт базу до 60 секунд — при первом запуске Postgres поднимается дольше сервера.
// Простаивающие соединения закрываем через 4 минуты: облачная база (Neon) засыпает через 5 и рвёт их сама.
func connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pc.MaxConnIdleTime = 4 * time.Minute
	pc.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err = pool.Ping(ctx); err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, err
		}
		slog.Info("жду базу данных…")
		time.Sleep(2 * time.Second)
	}
}
