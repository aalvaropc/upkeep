package lib

import "errors"

type Claims struct {
	Subject string
	Expires int64
	secret  string
}

type Token map[string]string

type Options struct {
	Strict bool
	cache  int
}

const (
	LevelLow Level = iota
	LevelHigh
)

type Level int

var ErrExpired = errors.New("expired")

func Parse(s string) (Token, error) { return nil, nil }

func Verify(a int) error { return nil }

func Legacy() {}

func Keys[K comparable, V any](m map[K]V) []K { return nil }

func (t Token) Get(key string) string { return t[key] }

func Wrap(v interface{}) interface{} { return v }
