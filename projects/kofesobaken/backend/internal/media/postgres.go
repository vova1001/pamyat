package media

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"kofesobaken/backend/internal/store"
)

// Postgres — фото в таблице media (переживают перезапуск на хостинге без диска).
type Postgres struct{ Q *store.Queries }

func (p Postgres) Save(ctx context.Context, name, contentType string, data []byte) error {
	if !ValidName(name) {
		return ErrNotFound
	}
	return p.Q.SaveMedia(ctx, store.SaveMediaParams{Name: name, ContentType: contentType, Data: data})
}

func (p Postgres) Open(ctx context.Context, name string) (File, error) {
	if !ValidName(name) {
		return File{}, ErrNotFound
	}
	r, err := p.Q.GetMedia(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	return File{ContentType: r.ContentType, Data: r.Data, ModTime: r.CreatedAt}, nil
}
