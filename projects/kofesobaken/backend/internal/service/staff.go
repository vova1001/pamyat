package service

import (
	"context"
	"errors"
	"net/mail"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"kofesobaken/backend/internal/store"
)

// Staff — человек с доступом в кабинет (для раздела «Сотрудники»).
type Staff struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Role               string     `json:"role"`
	Active             bool       `json:"active"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at"`
}

func staffOf(a store.Admin) Staff {
	return Staff{ID: a.ID, Name: a.Name, Email: a.Email, Role: a.Role, Active: a.Active, MustChangePassword: a.MustChangePassword, CreatedAt: a.CreatedAt, LastLoginAt: a.LastLoginAt}
}

type StaffCreate struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"` // staff (по умолчанию) или admin
}

type StaffUpdate struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Active bool   `json:"active"`
}

var (
	errEmailTaken = &ConflictError{Msg: "Эта почта уже есть в кабинете"}
	errSelf       = &ConflictError{Msg: "Себя здесь не изменить — свои почта и пароль меняются в «Профиле»"}
)

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func checkStaff(f fields, name, email string) {
	checkLen(f, "name", name, 1, 60, "Укажите имя — до 60 символов")
	checkEmail(f, "email", email)
}

func checkEmail(f fields, key, email string) {
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 120 {
		f.add(key, "Проверьте адрес почты")
	}
}

func (s *Service) ListStaff(ctx context.Context) ([]Staff, error) {
	rows, err := s.q.ListStaff(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Staff, 0, len(rows))
	for _, r := range rows {
		out = append(out, staffOf(r))
	}
	return out, nil
}

// CreateStaff — новый вход с временным паролем от администратора; при первом входе человек сменит его.
func (s *Service) CreateStaff(ctx context.Context, in StaffCreate) (Staff, error) {
	name, email := cleanText(in.Name), normEmail(in.Email)
	if in.Role == "" {
		in.Role = RoleStaff
	}
	f := fields{}
	checkStaff(f, name, email)
	checkPassword(f, "password", in.Password)
	if in.Role != RoleStaff && in.Role != RoleAdmin {
		f.add("role", "Выберите роль")
	}
	if err := f.err(); err != nil {
		return Staff{}, err
	}
	h, err := hashPassword(in.Password)
	if err != nil {
		return Staff{}, err
	}
	a, err := s.q.CreateStaff(ctx, store.CreateStaffParams{Email: email, PasswordHash: h, Name: name, Role: in.Role})
	if isUnique(err) {
		return Staff{}, errEmailTaken
	}
	if err != nil {
		return Staff{}, err
	}
	return staffOf(a), nil
}

// UpdateStaff меняет имя, почту и доступ. Отключённый сразу выходит из всех сессий.
// actor — кто меняет: себя отключить нельзя (так в кабинете всегда остаётся администратор).
func (s *Service) UpdateStaff(ctx context.Context, actor Account, id int64, in StaffUpdate) (Staff, error) {
	if id == actor.ID {
		return Staff{}, errSelf
	}
	name, email := cleanText(in.Name), normEmail(in.Email)
	f := fields{}
	checkStaff(f, name, email)
	if err := f.err(); err != nil {
		return Staff{}, err
	}
	var out store.Admin
	err := s.inTx(ctx, func(q *store.Queries) error {
		a, err := q.UpdateStaff(ctx, store.UpdateStaffParams{ID: id, Name: name, Email: email, Active: in.Active})
		if err != nil {
			return err
		}
		out = a
		if !a.Active {
			return q.DeleteAdminSessions(ctx, a.ID)
		}
		return nil
	})
	if isUnique(err) {
		return Staff{}, errEmailTaken
	}
	if err != nil {
		return Staff{}, notFound(err)
	}
	return staffOf(out), nil
}

// ResetStaffPassword — новый временный пароль (забыл свой). Все сессии этого человека закрываются.
func (s *Service) ResetStaffPassword(ctx context.Context, actor Account, id int64, password string) (Staff, error) {
	if id == actor.ID {
		return Staff{}, errSelf
	}
	f := fields{}
	checkPassword(f, "password", password)
	if err := f.err(); err != nil {
		return Staff{}, err
	}
	h, err := hashPassword(password)
	if err != nil {
		return Staff{}, err
	}
	var out store.Admin
	err = s.inTx(ctx, func(q *store.Queries) error {
		a, err := q.ResetStaffPassword(ctx, store.ResetStaffPasswordParams{ID: id, PasswordHash: h})
		if err != nil {
			return err
		}
		out = a
		return q.DeleteAdminSessions(ctx, a.ID)
	})
	if err != nil {
		return Staff{}, notFound(err)
	}
	return staffOf(out), nil
}
