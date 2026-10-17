package service

import (
	"context"

	"kofesobaken/backend/internal/store"
)

type MenuInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceKop    int64  `json:"price_kop"`
	Category    string `json:"category"`
	Photo       string `json:"photo"`
	Visible     *bool  `json:"visible"`
	Signature   bool   `json:"signature"`
}

func validCategory(c string) bool { return c == "black" || c == "milk" || c == "other" }

func (in *MenuInput) check() error {
	f := fields{}
	in.Name, in.Description, in.Photo = cleanText(in.Name), cleanText(in.Description), cleanText(in.Photo)
	checkLen(f, "name", in.Name, 1, 60, "Укажите название (до 60 символов)")
	checkLen(f, "description", in.Description, 0, 80, "Не больше 80 символов")
	if in.PriceKop <= 0 || in.PriceKop > 100_000_00 {
		f.add("price_kop", "Укажите цену")
	}
	if !validCategory(in.Category) {
		f.add("category", "Выберите раздел")
	}
	if in.Visible == nil {
		t := true
		in.Visible = &t
	}
	return f.err()
}

func (s *Service) ListMenu(ctx context.Context) ([]store.MenuItem, error) {
	return s.q.ListMenuItems(ctx)
}

func (s *Service) CreateMenuItem(ctx context.Context, in MenuInput) (store.MenuItem, error) {
	if err := in.check(); err != nil {
		return store.MenuItem{}, err
	}
	return s.q.CreateMenuItem(ctx, store.CreateMenuItemParams{
		Name: in.Name, Description: in.Description, PriceKop: in.PriceKop, Category: in.Category,
		Photo: in.Photo, Visible: *in.Visible, Signature: in.Signature,
	})
}

func (s *Service) UpdateMenuItem(ctx context.Context, id int64, in MenuInput) (store.MenuItem, error) {
	if err := in.check(); err != nil {
		return store.MenuItem{}, err
	}
	m, err := s.q.UpdateMenuItem(ctx, store.UpdateMenuItemParams{
		ID: id, Name: in.Name, Description: in.Description, PriceKop: in.PriceKop, Category: in.Category,
		Photo: in.Photo, Visible: *in.Visible, Signature: in.Signature,
	})
	return m, notFound(err)
}

func (s *Service) DeleteMenuItem(ctx context.Context, id int64) error {
	n, err := s.q.DeleteMenuItem(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
