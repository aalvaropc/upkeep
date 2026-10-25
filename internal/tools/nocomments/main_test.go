package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "clean.go", "package p\n\nvar s = \"// not a comment\"\n")
	write(t, dir, "embed.go", "package p\n\nimport \"embed\"\n\n//go:embed clean.go\nvar files embed.FS\n")
	write(t, dir, "line.go", "package p\n\n"+"/"+"/ a line comment\nvar a = 1\n")
	write(t, dir, "block.go", "package p\n\nvar b = 2 "+"/"+"* a block *"+"/\n")
	write(t, dir, "testdata/ignored.go", "package p\n\n"+"/"+"/ ignored\n")
	write(t, dir, ".hidden/ignored.go", "package p\n\n"+"/"+"/ ignored\n")
	write(t, dir, "notes.txt", "/"+"/ not go\n")

	var out bytes.Buffer
	found, err := scan([]string{dir}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if found != 2 {
		t.Fatalf("found %d comments, want 2; output:\n%s", found, out.String())
	}
	for _, want := range []string{"line.go:3:1", "block.go:3:11"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not mention %s:\n%s", want, out.String())
		}
	}
}

func TestScanParseError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.go", "package\n")
	if _, err := scan([]string{dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("scan of a broken file returned no error")
	}
}
