package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrMiss = errors.New("fixture not found")

type Store interface {
	Load(kind, key string, v any) error
	Save(kind, key string, v any) error
}

func Key(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type Dir struct {
	Path string
}

func name(kind, key string) (string, error) {
	for _, part := range []string{kind, key} {
		if part == "" || strings.ContainsAny(part, `/\`) || part == "." || part == ".." {
			return "", fmt.Errorf("invalid fixture name %q", part)
		}
	}
	return kind + "/" + key + ".json", nil
}

func (d Dir) file(kind, key string) (string, error) {
	rel, err := name(kind, key)
	if err != nil {
		return "", err
	}
	return filepath.Join(d.Path, filepath.FromSlash(rel)), nil
}

func (d Dir) Load(kind, key string, v any) error {
	path, err := d.file(kind, key)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s/%s", ErrMiss, kind, key)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (d Dir) Save(kind, key string, v any) error {
	path, err := d.file(kind, key)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fixture-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

var ErrReadOnly = errors.New("the fixture store is read-only")

type FS struct {
	FS fs.FS
}

func (f FS) Load(kind, key string, v any) error {
	rel, err := name(kind, key)
	if err != nil {
		return err
	}
	data, err := fs.ReadFile(f.FS, rel)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s/%s", ErrMiss, kind, key)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (FS) Save(kind, key string, _ any) error {
	return fmt.Errorf("%w: %s/%s", ErrReadOnly, kind, key)
}

type None struct{}

func (None) Load(kind, key string, _ any) error {
	return fmt.Errorf("%w: %s/%s", ErrMiss, kind, key)
}

func (None) Save(kind, key string, _ any) error {
	return fmt.Errorf("%w: %s/%s", ErrReadOnly, kind, key)
}

type Layered struct {
	Primary  Store
	Fallback Store
}

func (l Layered) Load(kind, key string, v any) error {
	err := l.Primary.Load(kind, key, v)
	if errors.Is(err, ErrMiss) && l.Fallback != nil {
		return l.Fallback.Load(kind, key, v)
	}
	return err
}

func (l Layered) Save(kind, key string, v any) error {
	return l.Primary.Save(kind, key, v)
}
