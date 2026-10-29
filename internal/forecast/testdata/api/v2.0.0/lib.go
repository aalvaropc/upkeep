package lib

import (
	"context"
	"errors"
)

type Claims struct {
	Subject   string
	ExpiresAt int64
	other     string
}

type Token map[string]string

type Options struct {
	Strict bool
	cache  map[string]int
}

const (
	LevelLow Level = iota
	LevelHigh
)

type Level int

var ErrExpired error = errors.New("expired")

func Parse(input string) (Token, error) { return nil, nil }

func Verify(ctx context.Context, a int) error { return nil }

func Keys[K comparable, V any](m map[K]V) []K { return nil }

func (t Token) Get(key, fallback string) string { return t[key] }

func Wrap(v any) any { return v }
