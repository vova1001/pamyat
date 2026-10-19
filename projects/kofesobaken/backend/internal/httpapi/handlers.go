package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"kofesobaken/backend/internal/media"
	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/service"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Ping(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable", "База данных недоступна", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) site(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Site(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

// ---------- оформление ----------

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	var in service.OrderInput
	if !decode(w, r, &in) {
		return
	}
	c, err := s.svc.CreateOrder(r.Context(), in)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) {
	var in service.BookingInput
	if !decode(w, r, &in) {
		return
	}
	c, err := s.svc.CreateBooking(r.Context(), in)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) createSignup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.SignupInput
	if !decode(w, r, &in) {
		return
	}
	c, err := s.svc.CreateSignup(r.Context(), id, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ---------- оплата ----------

func (s *Server) paymentStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathPID(w, r)
	if !ok {
		return
	}
	p, err := s.svc.Payment(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) paymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "Не удалось прочитать уведомление", nil)
		return
	}
	ev, err := s.pay.ParseWebhook(r.Context(), r.Header, body)
	if errors.Is(err, payments.ErrBadSignature) {
		slog.Warn("уведомление об оплате с неверной подписью", "ip", r.RemoteAddr)
		writeErr(w, http.StatusUnauthorized, "bad_signature", "Неверная подпись", nil)
		return
	}
	if errors.Is(err, payments.ErrIgnored) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"}) // 200 — иначе платёжная система будет повторять
		return
	}
	if err != nil {
		if !errors.Is(err, payments.ErrMalformed) {
			// Платёжная система не ответила — 502: уведомление придёт повторно.
			slog.Warn("уведомление об оплате: проверка не удалась", "err", err)
			writeErr(w, http.StatusBadGateway, "provider_unavailable", "Платёжная система недоступна", nil)
			return
		}
		writeErr(w, http.StatusBadRequest, "bad_request", "Неверное уведомление", nil)
		return
	}
	if err := s.svc.HandlePaymentEvent(r.Context(), ev); err != nil {
		var ce *service.ConflictError
		if errors.As(err, &ce) {
			slog.Warn("уведомление об оплате отклонено", "reason", ce.Msg, "payment_id", ev.PaymentID)
			writeErr(w, http.StatusBadRequest, "rejected", ce.Msg, nil)
			return
		}
		fail(w, r, err)
		return
	}
	slog.Info("уведомление об оплате принято", "payment_id", ev.PaymentID, "event", ev.Type)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) testPay(w http.ResponseWriter, r *http.Request) {
	id, ok := pathPID(w, r)
	if !ok {
		return
	}
	p, err := s.svc.TestPay(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) testCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathPID(w, r)
	if !ok {
		return
	}
	p, err := s.svc.TestCancel(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---------- вход ----------

type loginIn struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(service.SessionTTL.Seconds())})
}

func (s *Server) tooMany(w http.ResponseWriter) {
	writeErr(w, http.StatusTooManyRequests, "rate_limited", "Слишком много попыток — подождите 15 минут", nil)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in loginIn
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if s.loginLimit.Blocked(r.RemoteAddr) || s.emailLimit.Blocked(email) {
		s.tooMany(w)
		return
	}
	token, acc, err := s.svc.Login(r.Context(), email, in.Password)
	if err != nil {
		if errors.Is(err, service.ErrBadLogin) {
			s.loginLimit.Fail(r.RemoteAddr)
			s.emailLimit.Fail(email)
		}
		fail(w, r, err)
		return
	}
	s.setSession(w, token)
	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		_ = s.svc.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, current(r).acc)
}

type setupIn struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// setup — первый вход администратора по одноразовой ссылке: свой пароль и сразу вход.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.loginLimit.Blocked(r.RemoteAddr) {
		s.tooMany(w)
		return
	}
	var in setupIn
	if !decode(w, r, &in) {
		return
	}
	token, acc, err := s.svc.SetupPassword(r.Context(), in.Token, in.Password)
	if err != nil {
		if errors.Is(err, service.ErrSetupExpired) {
			s.loginLimit.Fail(r.RemoteAddr)
		}
		fail(w, r, err)
		return
	}
	s.setSession(w, token)
	writeJSON(w, http.StatusOK, acc)
}

// setupInfo — чья ссылка (почта для экрана первого входа); 410, если ссылка не действует.
func (s *Server) setupInfo(w http.ResponseWriter, r *http.Request) {
	if s.loginLimit.Blocked(r.RemoteAddr) {
		s.tooMany(w)
		return
	}
	var in setupIn
	if !decode(w, r, &in) {
		return
	}
	email, err := s.svc.SetupEmail(r.Context(), in.Token)
	if errors.Is(err, service.ErrSetupExpired) {
		s.loginLimit.Fail(r.RemoteAddr)
	}
	respond(w, r, http.StatusOK, map[string]string{"email": email}, err)
}

// forgot — «Забыли пароль?»: письмо со ссылкой (1 час). Ответ один и тот же, есть такая почта или нет;
// mail=false — почта не настроена, кабинет подскажет обратиться к администратору.
func (s *Server) forgot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	on := s.svc.MailConfigured()
	if on && email != "" && s.resetLimit.Allow(email) {
		// письмо — в фоне: время ответа не выдаёт, есть ли такая почта
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 20*time.Second)
			defer cancel()
			if err := s.svc.ForgotPassword(ctx, email); err != nil {
				slog.Error("письмо восстановления пароля", "err", err)
			}
		}()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"mail": on})
}

// call — кнопка «Позвонить» в Telegram (кнопки не принимают tel:): перенаправление на tel:<номер>.
// Принимаем только цифры и ведущий «+» — иначе не перенаправляем никуда.
func (s *Server) call(w http.ResponseWriter, r *http.Request) {
	p := chi.URLParam(r, "phone")
	if !phoneRe.MatchString(p) {
		writeErr(w, http.StatusNotFound, "not_found", "Не найдено", nil)
		return
	}
	http.Redirect(w, r, "tel:"+p, http.StatusFound)
}

var phoneRe = regexp.MustCompile(`^\+?[0-9]{5,15}$`)

type passwordIn struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	if s.loginLimit.Blocked(r.RemoteAddr) {
		s.tooMany(w)
		return
	}
	var in passwordIn
	if !decode(w, r, &in) {
		return
	}
	cur := current(r)
	if err := s.svc.ChangePassword(r.Context(), cur.acc, cur.token, in.CurrentPassword, in.NewPassword); err != nil {
		if errors.Is(err, service.ErrWrongPassword) {
			s.loginLimit.Fail(r.RemoteAddr)
		}
		fail(w, r, err)
		return
	}
	acc := cur.acc
	acc.MustChangePassword = false
	writeJSON(w, http.StatusOK, acc)
}

type emailIn struct {
	CurrentPassword string `json:"current_password"`
	Email           string `json:"email"`
}

// changeEmail — «Профиль»: своя почта (логин). Неверный текущий пароль считается как неверный вход.
func (s *Server) changeEmail(w http.ResponseWriter, r *http.Request) {
	if s.loginLimit.Blocked(r.RemoteAddr) {
		s.tooMany(w)
		return
	}
	var in emailIn
	if !decode(w, r, &in) {
		return
	}
	cur := current(r)
	acc, err := s.svc.ChangeEmail(r.Context(), cur.acc, cur.token, in.CurrentPassword, in.Email)
	if errors.Is(err, service.ErrWrongPassword) {
		s.loginLimit.Fail(r.RemoteAddr)
	}
	respond(w, r, http.StatusOK, acc, err)
}

// ---------- сотрудники (только администратор) ----------

func (s *Server) listStaff(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListStaff(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) createStaff(w http.ResponseWriter, r *http.Request) {
	var in service.StaffCreate
	if decode(w, r, &in) {
		v, err := s.svc.CreateStaff(r.Context(), in)
		respond(w, r, http.StatusCreated, v, err)
	}
}

func (s *Server) updateStaff(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	var in service.StaffUpdate
	if ok && decode(w, r, &in) {
		v, err := s.svc.UpdateStaff(r.Context(), current(r).acc, id, in)
		respond(w, r, http.StatusOK, v, err)
	}
}

type resetIn struct {
	Password string `json:"password"`
}

func (s *Server) resetStaffPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	var in resetIn
	if ok && decode(w, r, &in) {
		v, err := s.svc.ResetStaffPassword(r.Context(), current(r).acc, id, in.Password)
		respond(w, r, http.StatusOK, v, err)
	}
}

// ---------- кабинет ----------

// respond — общий вид простых обработчиков кабинета.
func respond[T any](w http.ResponseWriter, r *http.Request, status int, v T, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) listBookings(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListBookings(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) confirmBooking(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.svc.ConfirmBooking(r.Context(), id)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) cancelBooking(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.svc.CancelBooking(r.Context(), id)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListOrders(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) orderDone(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.svc.OrderDone(r.Context(), id)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.svc.CancelOrder(r.Context(), id)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) listMenu(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListMenu(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) createMenu(w http.ResponseWriter, r *http.Request) {
	var in service.MenuInput
	if decode(w, r, &in) {
		v, err := s.svc.CreateMenuItem(r.Context(), in)
		respond(w, r, http.StatusCreated, v, err)
	}
}

func (s *Server) updateMenu(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	var in service.MenuInput
	if ok && decode(w, r, &in) {
		v, err := s.svc.UpdateMenuItem(r.Context(), id, in)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) deleteMenu(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		if err := s.svc.DeleteMenuItem(r.Context(), id); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListEvents(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var in service.EventInput
	if decode(w, r, &in) {
		v, err := s.svc.CreateEvent(r.Context(), in)
		respond(w, r, http.StatusCreated, v, err)
	}
}

func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	var in service.EventInput
	if ok && decode(w, r, &in) {
		v, err := s.svc.UpdateEvent(r.Context(), id, in)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		if err := s.svc.DeleteEvent(r.Context(), id); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) listSignups(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.svc.ListSignups(r.Context(), id)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) getContent(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Content(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) putContent(w http.ResponseWriter, r *http.Request) {
	var in service.Content
	if decode(w, r, &in) {
		v, err := s.svc.PutContent(r.Context(), in)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Settings(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in service.Settings
	if decode(w, r, &in) {
		v, err := s.svc.PutSettings(r.Context(), in)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) testNotify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Channel string `json:"channel"`
	}
	if decode(w, r, &in) {
		v, err := s.svc.TestNotify(r.Context(), in.Channel)
		respond(w, r, http.StatusOK, v, err)
	}
}

func (s *Server) tgConnect(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.TelegramConnect(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) tgCheck(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.TelegramCheck(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) tgDisconnect(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.TelegramDisconnect(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListNotifications(r.Context())
	respond(w, r, http.StatusOK, v, err)
}

// ---------- фото ----------

const maxUpload = 8 << 20

var imageExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "Фото больше 8 МБ — уменьшите и попробуйте снова", nil)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "validation", "Выберите фото", nil)
		return
	}
	defer f.Close()
	if hdr.Size > maxUpload {
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "Фото больше 8 МБ — уменьшите и попробуйте снова", nil)
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext, ok := imageExt[http.DetectContentType(head[:n])]
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, "validation", "Нужно фото в формате JPG, PNG или WebP", nil)
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		fail(w, r, err)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, maxUpload+1))
	if err != nil {
		fail(w, r, err)
		return
	}
	rnd := make([]byte, 12)
	_, _ = rand.Read(rnd)
	name := time.Now().UTC().Format("20060102") + "-" + hex.EncodeToString(rnd) + ext
	if err := s.media.Save(r.Context(), name, http.DetectContentType(head[:n]), data); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": "/uploads/" + name})
}

// serveUpload отдаёт загруженное фото из хранилища; имена — только выданные сервером, без списка файлов.
func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	f, err := s.media.Open(r.Context(), name)
	if errors.Is(err, media.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("фото не отдаётся", "err", err, "name", name)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, f.ModTime, bytes.NewReader(f.Data))
}

// ---------- вопросы гостей ----------

func (s *Server) createQuestion(w http.ResponseWriter, r *http.Request) {
	var in service.QuestionInput
	if !decode(w, r, &in) {
		return
	}
	if _, err := s.svc.CreateQuestion(r.Context(), in); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) listQuestions(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ListQuestions(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) answerQuestion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Answer string `json:"answer"`
		Send   bool   `json:"send"`
	}
	if !decode(w, r, &in) {
		return
	}
	q, err := s.svc.AnswerQuestion(r.Context(), id, in.Answer, in.Send)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}
