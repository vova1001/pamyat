// Сервер API КофеСобакен: настройки из окружения, миграции, подготовка БД, HTTP.
//
//	server             — запуск
//	server admin-link  — новая одноразовая ссылка входа администратора (забыл пароль): пароль задаётся заново
//	ADMIN_LINK_ON_START=true — то же при каждом старте (хостинг без консоли, например бесплатный Render)
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

	pay := payments.NewTestProvider(cfg.PublicURL, cfg.WebhookURL, cfg.WebhookSecret)
	channels := []notify.Channel{notify.LogChannel{Channel_: "email"}, notify.LogChannel{Channel_: "telegram"}}
	svc := service.New(pool, pay, channels, cfg.Location)
	svc.SetAdminPasswordTemporary(cfg.AdminPassTemp)
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
	}
	if cmdLink {
		return nil
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
	slog.Info("сервер запущен", "addr", cfg.HTTPAddr, "payments", pay.Name(), "photos", cfg.MediaStore)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// logSetupLink — ссылка первого входа: по ней администратор сам задаёт пароль. В базе — только хэш кода.
func logSetupLink(cfg config.Config, code string) {
	slog.Warn("ссылка входа администратора: откройте её и придумайте пароль (одноразовая, 72 часа)",
		"email", cfg.AdminEmail, "link", cfg.PublicURL+"/admin#setup/"+code)
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
