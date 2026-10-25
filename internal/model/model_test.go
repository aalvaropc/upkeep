package model

import (
	"context"
	"errors"
	"testing"
)

type fake string

func (f fake) Complete(context.Context, Request) (Response, error) {
	return Response{Message: Message{Role: "assistant", Content: string(f)}}, nil
}

func TestRouterFor(t *testing.T) {
	r := NewRouter()
	r.Set(Super, "super-model", fake("super"))
	r.Set(Nano, "nano-model", fake("nano"))

	name, m, err := r.For(Nano)
	if err != nil || name != "nano-model" || m != fake("nano") {
		t.Fatalf("For(Nano) = %q, %v, %v", name, m, err)
	}
	name, m, err = r.For(Ultra)
	if err != nil || name != "super-model" || m != fake("super") {
		t.Fatalf("For(Ultra) without Ultra = %q, %v, %v; want the Super route", name, m, err)
	}
	r.Set(Ultra, "ultra-model", fake("ultra"))
	if name, _, _ := r.For(Ultra); name != "ultra-model" {
		t.Fatalf("For(Ultra) = %q, want ultra-model", name)
	}
	if _, _, err := NewRouter().For(Super); err == nil {
		t.Fatal("For on an empty router returned no error")
	}
}

func TestRouterLargest(t *testing.T) {
	r := NewRouter()
	if _, _, err := r.Largest(); err == nil {
		t.Fatal("Largest on an empty router returned no error")
	}
	r.Set(Nano, "nano-model", fake("nano"))
	if role, name, _ := r.Largest(); role != Nano || name != "nano-model" {
		t.Fatalf("Largest = %s %s", role, name)
	}
	r.Set(Ultra, "ultra-model", fake("ultra"))
	if role, name, _ := r.Largest(); role != Ultra || name != "ultra-model" {
		t.Fatalf("Largest = %s %s", role, name)
	}
}

func TestUsageTotal(t *testing.T) {
	u := Usage{PromptTokens: 100, CompletionTokens: 40, ReasoningTokens: 25}
	if u.Total() != 140 {
		t.Fatalf("Total() = %d, want 140 (reasoning tokens are part of the completion)", u.Total())
	}
}

func TestStubsSayNotImplemented(t *testing.T) {
	for _, m := range []Model{&Recorder{}, &Replayer{}} {
		if _, err := m.Complete(t.Context(), Request{}); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%T.Complete error = %v, want ErrUnsupported", m, err)
		}
	}
}
