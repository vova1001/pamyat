package media

import (
	"context"
	"errors"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
)

// Disk — фото файлами в папке (том uploads в docker compose).
type Disk struct{ Dir string }

func (d Disk) Save(_ context.Context, name, _ string, data []byte) error {
	if !ValidName(name) {
		return ErrNotFound
	}
	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d.Dir, name), data, 0o644)
}

func (d Disk) Open(_ context.Context, name string) (File, error) {
	if !ValidName(name) {
		return File{}, ErrNotFound
	}
	p := filepath.Join(d.Dir, name)
	st, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return File{}, err
	}
	return File{ContentType: mime.TypeByExtension(filepath.Ext(name)), Data: b, ModTime: st.ModTime()}, nil
}
