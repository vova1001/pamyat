// Package httpapi — обработчики REST API по контракту api/openapi.yaml.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"kofesobaken/backend/internal/media"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
)

const cookieName = "ks_session"

type Server struct {
	svc          *service.Service
	pay          payments.Provider
	media        media.Store
	cookieSecure bool
	formLimit    *RateLimiter
	loginLimit   *RateLimiter // неверные пароли с одного адреса
	emailLimit   *RateLimiter // неверные пароли к одной почте — с любых адресов
	resetLimit   *RateLimiter // письма «забыли пароль» на одну почту
	askLimit     *RateLimiter // вопросы с сайта с одного адреса
}

type Options struct {
	UploadDir    string      // фото на диске, если Media не задан
	Media        media.Store // где хранить фото (диск, PostgreSQL, потом S3)
	CookieSecure bool
}

func New(svc *service.Service, pay payments.Provider, o Options) *Server {
	if o.Media == nil {
		o.Media = media.Disk{Dir: o.UploadDir}
	}
	return &Server{
		svc: svc, pay: pay, media: o.Media, cookieSecure: o.CookieSecure,
		formLimit:  NewRateLimiter(10, time.Minute),   // 10 оформлений в минуту с одного адреса
		loginLimit: NewRateLimiter(8, 15*time.Minute), // 8 неверных паролей за 15 минут
		emailLimit: NewRateLimiter(20, 15*time.Minute),
		resetLimit: NewRateLimiter(3, time.Hour), // 3 письма в час на одну почту
		askLimit:   NewRateLimiter(5, time.Hour), // 5 вопросов в час с одного адреса
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(realIP, middleware.RequestID, logRequests, middleware.Recoverer)

	r.Get("/healthz", s.health)
	r.Get("/uploads/{name}", s.serveUpload)

	r.Route("/api", func(r chi.Router) {
		r.Get("/healthz", s.health)
		r.Get("/site", s.site)
		r.Get("/call/{phone}", s.call)
		r.With(s.formLimit.Middleware).Post("/orders", s.createOrder)
		r.With(s.formLimit.Middleware).Post("/bookings", s.createBooking)
		r.With(s.formLimit.Middleware).Post("/events/{id}/signups", s.createSignup)
		r.With(s.askLimit.Middleware).Post("/questions", s.createQuestion)

		r.Get("/payments/{pid}", s.paymentStatus)
		r.Post("/payments/webhook", s.paymentWebhook)
		if _, ok := s.pay.(*payments.TestProvider); ok {
			r.Post("/payments/test/{pid}/pay", s.testPay)
			r.Post("/payments/test/{pid}/cancel", s.testCancel)
		}

		r.Route("/admin", func(r chi.Router) {
			r.Post("/login", s.login)
			r.Post("/logout", s.logout)
			r.Post("/setup", s.setup)
			r.Post("/setup/info", s.setupInfo)
			r.With(s.formLimit.Middleware).Post("/forgot", s.forgot)
			r.Group(func(r chi.Router) {
				r.Use(s.requireLogin)
				// доступно и с временным паролем — чтобы сменить его
				r.Get("/me", s.me)
				r.Post("/password", s.changePassword)

				// сотрудник и администратор
				r.Group(func(r chi.Router) {
					r.Use(requirePasswordSet)
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
					r.Post("/uploads", s.upload)
					r.Get("/questions", s.listQuestions)
					r.Post("/questions/{id}/answer", s.answerQuestion)
				})

				// только администратор: тексты и фото сайта, уведомления, сотрудники
				r.Group(func(r chi.Router) {
					r.Use(requirePasswordSet, requireRole(service.RoleAdmin))
					r.Get("/content", s.getContent)
					r.Put("/content", s.putContent)
					r.Get("/settings", s.getSettings)
					r.Put("/settings", s.putSettings)
					r.Post("/notify/test", s.testNotify)
					r.Post("/telegram/connect", s.tgConnect)
					r.Post("/telegram/check", s.tgCheck)
					r.Post("/telegram/disconnect", s.tgDisconnect)
					r.Get("/notifications", s.listNotifications)
					r.Get("/staff", s.listStaff)
					r.Post("/staff", s.createStaff)
					r.Put("/staff/{id}", s.updateStaff)
					r.Post("/staff/{id}/password", s.resetStaffPassword)
				})
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
	case errors.Is(err, service.ErrForbidden):
		writeErr(w, http.StatusForbidden, "forbidden", "Этот раздел — только для администратора", nil)
	case errors.Is(err, service.ErrMustChangePassword):
		writeErr(w, http.StatusForbidden, "password_change_required", "Сначала смените временный пароль", nil)
	case errors.Is(err, service.ErrWrongPassword):
		writeErr(w, http.StatusUnprocessableEntity, "validation", "Неверный текущий пароль", map[string]string{"current_password": "Неверный текущий пароль"})
	case errors.Is(err, service.ErrSetupExpired):
		writeErr(w, http.StatusGone, "setup_expired", "Ссылка устарела или уже использована — попросите в студии новую", nil)
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
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "ms", time.Since(start).Milliseconds(), "ip", r.RemoteAddr, "request_id", middleware.GetReqID(r.Context()))
	})
}

type ctxKey struct{}

// realIP — адрес посетителя для ограничений частоты. Доверяем только X-Real-IP, который ставит наш nginx
// (перезаписывает его у каждого запроса); остальные заголовки посетитель может подделать. Без nginx — адрес
// соединения без порта.
func realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := strings.TrimSpace(r.Header.Get("X-Real-IP"))
		if net.ParseIP(ip) == nil {
			ip = r.RemoteAddr
			if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				ip = h
			}
		}
		r.RemoteAddr = ip
		next.ServeHTTP(w, r)
	})
}

type session struct {
	acc   service.Account
	token string
}

func current(r *http.Request) session { return r.Context().Value(ctxKey{}).(session) }

// requireLogin — любой вошедший (администратор или сотрудник); отключённый сотрудник — 401.
func (s *Server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			fail(w, r, service.ErrUnauthorized)
			return
		}
		acc, err := s.svc.Session(r.Context(), c.Value)
		if err != nil {
			fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, session{acc: acc, token: c.Value})))
	})
}

// requirePasswordSet — с временным паролем доступны только /me и смена пароля.
func requirePasswordSet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if current(r).acc.MustChangePassword {
			fail(w, r, service.ErrMustChangePassword)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireRole — права по роли проверяются на сервере, не только скрытием разделов в кабинете.
func requireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if current(r).acc.Role != role {
				fail(w, r, service.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
