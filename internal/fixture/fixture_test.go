package fixture

import (
	"errors"
	"testing"
)

type request struct {
	Model    string            `json:"model"`
	Messages []string          `json:"messages"`
	Meta     map[string]string `json:"meta"`
}

func TestKeyIsStable(t *testing.T) {
	a := request{Model: "m", Messages: []string{"hi"}, Meta: map[string]string{"b": "2", "a": "1"}}
	b := request{Model: "m", Messages: []string{"hi"}, Meta: map[string]string{"a": "1", "b": "2"}}
	ka, err := Key(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := Key(b)
	if err != nil {
		t.Fatal(err)
	}
	if ka != kb || len(ka) != 64 {
		t.Fatalf("keys = %q and %q, want equal 64-character hashes", ka, kb)
	}
	kc, _ := Key(request{Model: "m", Messages: []string{"hello"}})
	if kc == ka {
		t.Fatal("different requests produced the same key")
	}
	if _, err := Key(func() {}); err == nil {
		t.Fatal("Key of a function returned no error")
	}
}

func TestDirRoundTrip(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	want := request{Model: "m", Messages: []string{"a", "b"}}
	if err := d.Save("model", "abc", want); err != nil {
		t.Fatal(err)
	}
	var got request
	if err := d.Load("model", "abc", &got); err != nil {
		t.Fatal(err)
	}
	if got.Model != want.Model || len(got.Messages) != 2 {
		t.Fatalf("loaded %+v", got)
	}
	if err := d.Save("model", "abc", request{Model: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Load("model", "abc", &got); err != nil || got.Model != "n" {
		t.Fatalf("after overwrite: %+v %v", got, err)
	}
}

func TestDirMiss(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	var v request
	if err := d.Load("model", "missing", &v); !errors.Is(err, ErrMiss) {
		t.Fatalf("Load of a missing fixture: %v, want ErrMiss", err)
	}
}

func TestDirRejectsPaths(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	for _, name := range [][2]string{{"../x", "k"}, {"model", "a/b"}, {"", "k"}, {"model", ".."}} {
		if err := d.Save(name[0], name[1], 1); err == nil {
			t.Errorf("Save(%q, %q) accepted a path", name[0], name[1])
		}
	}
}
