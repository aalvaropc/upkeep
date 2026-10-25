package forecast

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPackageNameFollowsGoConventions(t *testing.T) {
	for path, want := range map[string]string{
		"github.com/golang-jwt/jwt/v5": "jwt",
		"github.com/rs/cors":           "cors",
		"gopkg.in/yaml.v3":             "yaml",
		"github.com/oklog/ulid/v2":     "ulid",
		"example.com/lib":              "lib",
	} {
		if got := packageName(path); got != want {
			t.Errorf("packageName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestUsesListsTheIdentifiersTheProjectTakesFromTheModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"auth/token.go": `package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	jwt.StandardClaims
}

func sign() (string, error) {
	fmt.Println("x")
	return jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{}).SignedString(nil)
}
`,
		"aliased.go": `package main

import j "github.com/golang-jwt/jwt/v5"

var _ = j.ErrTokenExpired
`,
		"shadow.go": `package main

import "github.com/golang-jwt/jwt/v5"

func f() {
	_ = jwt.Keyfunc(nil)
	jwt := struct{ Hidden int }{}
	_ = jwt.Hidden
}
`,
		"other.go":     "package main\n\nimport \"github.com/rs/cors\"\n\nvar _ = cors.New\n",
		"x_test.go":    "package main\n\nimport \"github.com/golang-jwt/jwt/v5\"\n\nvar _ = jwt.OnlyInTests\n",
		"vendor/v.go":  "package v\n\nimport \"github.com/golang-jwt/jwt/v5\"\n\nvar _ = jwt.Vendored\n",
		"broken.go":    "package {\n",
		"readme.txt":   "jwt.NotCode",
		".hidden/h.go": "package h\n\nimport \"github.com/golang-jwt/jwt/v5\"\n\nvar _ = jwt.Hidden\n",
	})
	got := uses(dir, "github.com/golang-jwt/jwt/v5")
	want := []string{"j.ErrTokenExpired", "jwt.Keyfunc", "jwt.NewWithClaims", "jwt.SigningMethodHS256", "jwt.StandardClaims"}
	if !slices.Equal(got, want) {
		t.Fatalf("uses = %v, want %v", got, want)
	}
	if uses("", "x") != nil || uses(filepath.Join(dir, "missing"), "x") != nil {
		t.Fatal("no directory means no identifiers")
	}
}

func TestUsesIsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("package a\n\nimport \"example.com/lib\"\n\nfunc f() {\n")
	for i := range 100 {
		b.WriteString("\t_ = lib.Name" + strings.Repeat("x", i%7) + string(rune('A'+i%26)) + string(rune('a'+i/26)) + "\n")
	}
	b.WriteString("}\n")
	dir := writeTree(t, map[string]string{"a.go": b.String()})
	if got := uses(dir, "example.com/lib"); len(got) != maxSymbolsListed {
		t.Fatalf("%d identifiers listed, want the cap of %d", len(got), maxSymbolsListed)
	}
}
