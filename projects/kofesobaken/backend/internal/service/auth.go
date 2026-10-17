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

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"kofesobaken/backend/internal/store"
)

const SessionTTL = 30 * 24 * time.Hour

var ErrBadLogin = errors.New("неверная почта или пароль")

// dummyHash — чтобы время ответа не выдавало, есть ли такая почта.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// Login проверяет почту и пароль и возвращает токен сессии для cookie.
func (s *Service) Login(ctx context.Context, email, password string) (token string, adminEmail string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	a, err := s.q.GetAdminByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", "", ErrBadLogin
	}
	if err != nil {
		return "", "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return "", "", ErrBadLogin
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	if err := s.q.CreateSession(ctx, store.CreateSessionParams{TokenHash: hashToken(token), AdminID: a.ID, ExpiresAt: s.now().Add(SessionTTL)}); err != nil {
		return "", "", err
	}
	_ = s.q.DeleteExpiredSessions(ctx)
	return token, a.Email, nil
}

// SessionEmail возвращает почту владельца по токену или ErrUnauthorized.
func (s *Service) SessionEmail(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrUnauthorized
	}
	row, err := s.q.GetSession(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	return row.Email, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, hashToken(token))
}
