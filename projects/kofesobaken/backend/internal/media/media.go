// Package media — где лежат фото из кабинета. Один интерфейс, хранилище выбирается в main.go (MEDIA_STORE):
// диск (docker compose, том uploads) или PostgreSQL (хостинг без постоянного диска). Переезд в S3 — ещё одна
// реализация Store, остальной код не меняется.
package media

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var ErrNotFound = errors.New("фото не найдено")

type Store interface {
	Save(ctx context.Context, name, contentType string, data []byte) error
	Open(ctx context.Context, name string) (File, error)
}

type File struct {
	ContentType string
	Data        []byte
	ModTime     time.Time
}

// ValidName — только имена, которые выдаёт сервер (дата-случайное.расширение): без путей и подстановок.
var validName = regexp.MustCompile(`^[0-9]{8}-[0-9a-f]{24}\.(jpg|png|webp)$`)

func ValidName(n string) bool { return validName.MatchString(n) }
