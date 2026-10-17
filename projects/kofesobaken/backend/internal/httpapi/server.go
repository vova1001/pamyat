// Package httpapi — обработчики REST API по контракту api/openapi.yaml.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
)

const cookieName = "ks_session"

type Server struct {
	svc          *service.Service
	pay          payments.Provider
	uploadDir    string
	cookieSecure bool
	formLimit    *RateLimiter
	loginLimit   *RateLimiter
}

type Options struct {
	UploadDir    string
	CookieSecure bool
}

func New(svc *service.Service, pay payments.Provider, o Options) *Server {
	return &Server{
		svc: svc, pay: pay, uploadDir: o.UploadDir, cookieSecure: o.CookieSecure,
		formLimit:  NewRateLimiter(10, time.Minute),   // 10 оформлений в минуту с одного адреса
		loginLimit: NewRateLimiter(8, 15*time.Minute), // 8 неверных паролей за 15 минут
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.RequestID, logRequests, middleware.Recoverer)

	r.Get("/healthz", s.health)
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", uploadsHandler(s.uploadDir)))

	r.Route("/api", func(r chi.Router) {
		r.Get("/healthz", s.health)
		r.Get("/site", s.site)
		r.With(s.formLimit.Middleware).Post("/orders", s.createOrder)
		r.With(s.formLimit.Middleware).Post("/bookings", s.createBooking)
		r.With(s.formLimit.Middleware).Post("/events/{id}/signups", s.createSignup)

		r.Get("/payments/{pid}", s.paymentStatus)
		r.Post("/payments/webhook", s.paymentWebhook)
		if _, ok := s.pay.(*payments.TestProvider); ok {
			r.Post("/payments/test/{pid}/pay", s.testPay)
			r.Post("/payments/test/{pid}/cancel", s.testCancel)
		}

		r.Route("/admin", func(r chi.Router) {
			r.Post("/login", s.login)
			r.Post("/logout", s.logout)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/me", s.me)
				r.Get("/bookings", s.listBookings)
				r.Post("/bookings/{id}/confirm", s.confirmBooking)
				r.Post("/bookings/{id}/cancel", s.cancelBooking)
				r.Get("/orders", s.listOrders)
				r.Post("/orders/{id}/done", s.orderDone)
				r.Post("/orders/{id}/cancel", s.cancelOrder)
				r.Get("/menu", s.listMenu)
				r.Post("/menu", s.createMenu)
				r.Put("/menu/{id}", s.updateMenu)
				r.Delete("/menu/{id}", s.deleteMenu)
				r.Get("/events", s.listEvents)
				r.Post("/events", s.createEvent)
				r.Put("/events/{id}", s.updateEvent)
				r.Delete("/events/{id}", s.deleteEvent)
				r.Get("/events/{id}/signups", s.listSignups)
				r.Get("/content", s.getContent)
				r.Put("/content", s.putContent)
				r.Get("/settings", s.getSettings)
				r.Put("/settings", s.putSettings)
				r.Post("/notify/test", s.testNotify)
				r.Post("/telegram/connect", s.tgConnect)
				r.Post("/telegram/disconnect", s.tgDisconnect)
				r.Get("/notifications", s.listNotifications)
				r.Post("/uploads", s.upload)
			})
		})
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeErr(w, http.StatusNotFound, "not_found", "Не найдено", nil)
		})
	})
	return r
}

// ---------- ответы ----------

type errBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string, f map[string]string) {
	var b errBody
	b.Error.Code, b.Error.Message, b.Error.Fields = code, msg, f
	writeJSON(w, status, b)
}

// fail переводит ошибку сервиса в понятный ответ; внутренности наружу не уходят.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve *service.ValidationError
	var ce *service.ConflictError
	switch {
	case errors.As(err, &ve):
		writeErr(w, http.StatusUnprocessableEntity, "validation", "Проверьте поля формы", ve.Fields)
	case errors.As(err, &ce):
		writeErr(w, http.StatusConflict, "conflict", ce.Msg, nil)
	case errors.Is(err, service.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "Не найдено", nil)
	case errors.Is(err, service.ErrUnauthorized):
		writeErr(w, http.StatusUnauthorized, "unauthorized", "Войдите в кабинет", nil)
	case errors.Is(err, service.ErrBadLogin):
		writeErr(w, http.StatusUnauthorized, "bad_login", "Неверная почта или пароль", nil)
	default:
		slog.Error("ошибка запроса", "err", err, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()))
		writeErr(w, http.StatusInternalServerError, "internal", "Что-то пошло не так. Попробуйте ещё раз", nil)
	}
}

// decode читает JSON-тело: только application/json (защита от отправки чужой формой), до 64 КБ.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "bad_request", "Нужен JSON", nil)
		return false
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "Не удалось прочитать данные формы", nil)
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusNotFound, "not_found", "Не найдено", nil)
		return 0, false
	}
	return id, true
}

func pathPID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "pid"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "Не найдено", nil)
		return uuid.Nil, false
	}
	return id, true
}

// ---------- middleware ----------

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/healthz" {
			return
		}
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}

type ctxKey struct{}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			fail(w, r, service.ErrUnauthorized)
			return
		}
		email, err := s.svc.SessionEmail(r.Context(), c.Value)
		if err != nil {
			fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, email)))
	})
}
