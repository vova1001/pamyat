package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"kofesobaken/backend/internal/store"
)

const (
	SessionTTL = 30 * 24 * time.Hour
	// SetupTTL — сколько действует ссылка первого входа администратора.
	SetupTTL = 72 * time.Hour

	RoleAdmin = "admin"
	RoleStaff = "staff"
)

var (
	ErrBadLogin = errors.New("неверная почта или пароль")
	// ErrForbidden — вошёл, но раздел не для этой роли.
	ErrForbidden = errors.New("нет доступа")
	// ErrMustChangePassword — вошёл с временным паролем: сначала смена пароля.
	ErrMustChangePassword = errors.New("нужно сменить временный пароль")
	// ErrWrongPassword — неверный текущий пароль при смене.
	ErrWrongPassword = errors.New("неверный текущий пароль")
	// ErrSetupExpired — ссылка первого входа неверна, уже использована или устарела.
	ErrSetupExpired = errors.New("ссылка устарела")
)

// Account — кто вошёл в кабинет.
type Account struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

func (a Account) IsAdmin() bool { return a.Role == RoleAdmin }

func accountOf(a store.Admin) Account {
	return Account{ID: a.ID, Email: a.Email, Name: a.Name, Role: a.Role, MustChangePassword: a.MustChangePassword}
}

// dummyHash — чтобы время ответа не выдавало, есть ли такая почта.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// checkPassword — правила нового пароля: от 8 символов, не больше 72 байт (предел bcrypt).
func checkPassword(f fields, key, pw string) {
	switch {
	case strings.TrimSpace(pw) == "" || utf8.RuneCountInString(pw) < 8:
		f.add(key, "Пароль — не короче 8 символов")
	case len(pw) > 72:
		f.add(key, "Пароль слишком длинный — до 72 символов")
	}
}

func hashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func (s *Service) newSession(ctx context.Context, adminID int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.q.CreateSession(ctx, store.CreateSessionParams{TokenHash: hashToken(token), AdminID: adminID, ExpiresAt: s.now().Add(SessionTTL)}); err != nil {
		return "", err
	}
	_ = s.q.TouchLogin(ctx, adminID)
	_ = s.q.DeleteExpiredSessions(ctx)
	return token, nil
}

// Login проверяет почту и пароль и возвращает токен сессии для cookie.
// Отключённый сотрудник и ещё не заданный пароль — тот же ответ, что и неверный пароль.
func (s *Service) Login(ctx context.Context, email, password string) (string, Account, error) {
	a, err := s.q.GetAdminByEmail(ctx, normEmail(email))
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", Account{}, ErrBadLogin
	}
	if err != nil {
		return "", Account{}, err
	}
	if a.PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", Account{}, ErrBadLogin
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil || !a.Active {
		return "", Account{}, ErrBadLogin
	}
	token, err := s.newSession(ctx, a.ID)
	if err != nil {
		return "", Account{}, err
	}
	return token, accountOf(a), nil
}

// Session возвращает того, кто вошёл, по токену или ErrUnauthorized (нет сессии, истекла, сотрудник отключён).
func (s *Service) Session(ctx context.Context, token string) (Account, error) {
	if token == "" {
		return Account{}, ErrUnauthorized
	}
	row, err := s.q.GetSession(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrUnauthorized
	}
	if err != nil {
		return Account{}, err
	}
	return Account{ID: row.AdminID, Email: row.Email, Name: row.Name, Role: row.Role, MustChangePassword: row.MustChangePassword}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, hashToken(token))
}

// ChangePassword — смена своего пароля; остальные сессии этого человека закрываются.
// Сразу после входа с временным паролем текущий не спрашиваем (его только что проверили при входе);
// в остальных случаях нужен текущий пароль.
func (s *Service) ChangePassword(ctx context.Context, acc Account, sessionToken, current, next string) error {
	a, err := s.q.GetAdminByID(ctx, acc.ID)
	if err != nil {
		return notFound(err)
	}
	if !a.MustChangePassword && bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(current)) != nil {
		return ErrWrongPassword
	}
	f := fields{}
	checkPassword(f, "new_password", next)
	if f["new_password"] == "" && bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(next)) == nil {
		f.add("new_password", "Новый пароль должен отличаться от прежнего")
	}
	if err := f.err(); err != nil {
		return err
	}
	h, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q *store.Queries) error {
		if err := q.SetOwnPassword(ctx, store.SetOwnPasswordParams{ID: a.ID, PasswordHash: h}); err != nil {
			return err
		}
		return q.DeleteOtherSessions(ctx, store.DeleteOtherSessionsParams{AdminID: a.ID, TokenHash: hashToken(sessionToken)})
	})
}

// IssueSetupLink создаёт администратору одноразовый код первого входа (старый код перестаёт действовать).
// В базе — только хэш кода; сам код показывается один раз (в логе сервера) ссылкой /admin#setup/<код>.
func (s *Service) IssueSetupLink(ctx context.Context, email string) (string, error) {
	a, err := s.q.GetAdminByEmail(ctx, normEmail(email))
	if err != nil {
		return "", notFound(err)
	}
	if a.Role != RoleAdmin {
		return "", ErrForbidden
	}
	code, err := randomToken()
	if err != nil {
		return "", err
	}
	exp := s.now().Add(SetupTTL)
	if err := s.q.SetSetupToken(ctx, store.SetSetupTokenParams{ID: a.ID, SetupTokenHash: pgtype.Text{String: hashToken(code), Valid: true}, SetupExpiresAt: &exp}); err != nil {
		return "", err
	}
	return code, nil
}

// SetupEmail — чья ссылка первого входа (для подсказки на экране); ErrSetupExpired, если ссылка не действует.
func (s *Service) SetupEmail(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrSetupExpired
	}
	a, err := s.q.GetAdminBySetupToken(ctx, pgtype.Text{String: hashToken(code), Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSetupExpired
	}
	return a.Email, err
}

// SetupPassword — первый вход администратора по одноразовой ссылке: свой пароль и сразу вход.
func (s *Service) SetupPassword(ctx context.Context, code, password string) (string, Account, error) {
	if code == "" {
		return "", Account{}, ErrSetupExpired
	}
	a, err := s.q.GetAdminBySetupToken(ctx, pgtype.Text{String: hashToken(code), Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Account{}, ErrSetupExpired
	}
	if err != nil {
		return "", Account{}, err
	}
	f := fields{}
	checkPassword(f, "password", password)
	if err := f.err(); err != nil {
		return "", Account{}, err
	}
	h, err := hashPassword(password)
	if err != nil {
		return "", Account{}, err
	}
	if err := s.inTx(ctx, func(q *store.Queries) error {
		if err := q.SetOwnPassword(ctx, store.SetOwnPasswordParams{ID: a.ID, PasswordHash: h}); err != nil {
			return err
		}
		return q.DeleteAdminSessions(ctx, a.ID)
	}); err != nil {
		return "", Account{}, err
	}
	token, err := s.newSession(ctx, a.ID)
	if err != nil {
		return "", Account{}, err
	}
	a.MustChangePassword = false
	return token, accountOf(a), nil
}
