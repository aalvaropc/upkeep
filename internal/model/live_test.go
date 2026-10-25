package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
)

func liveClient(t *testing.T) (*TokenFactory, config.Config) {
	t.Helper()
	if os.Getenv("UPKEEP_LIVE_TESTS") != "1" {
		t.Skip("set UPKEEP_LIVE_TESTS=1 and NEBIUS_API_KEY to call Token Factory")
	}
	cfg, err := config.Load(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NebiusAPIKey.IsSet() {
		t.Skip("NEBIUS_API_KEY is not set")
	}
	return NewTokenFactory(cfg.NebiusBaseURL, cfg.NebiusAPIKey), cfg
}

func TestLiveCompletionAndPrices(t *testing.T) {
	tf, cfg := liveClient(t)
	ctx := t.Context()

	book, err := tf.FetchPrices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{cfg.Models.Ultra, cfg.Models.Super, cfg.Models.Nano} {
		if _, ok := book[name]; !ok {
			t.Errorf("the catalog has no price for %s", name)
		}
	}

	off := false
	resp, err := tf.Complete(ctx, Request{
		Model:     cfg.Models.Nano,
		MaxTokens: 200,
		Reasoning: &off,
		Messages:  []Message{{Role: "user", Content: "Reply with the single word: pong"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(resp.Message.Content), "pong") {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.Usage.PromptTokens == 0 || resp.Usage.CompletionTokens == 0 || resp.Latency <= 0 || resp.Latency > time.Minute {
		t.Errorf("usage %+v latency %v", resp.Usage, resp.Latency)
	}
	t.Logf("nano: %v, %+v", resp.Latency.Round(time.Millisecond), resp.Usage)
}

func TestLiveToolCallRoundTrip(t *testing.T) {
	tf, cfg := liveClient(t)
	ctx := t.Context()
	tools := []Tool{{
		Name:        "read_file",
		Description: "Read a file of the repository.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}}
	off := false
	msgs := []Message{
		{Role: "system", Content: "You inspect a repository. Call read_file before answering questions about a file."},
		{Role: "user", Content: "What package does main.go declare? Read it first."},
	}
	first, err := tf.Complete(ctx, Request{Model: cfg.Models.Super, Messages: msgs, Tools: tools, MaxTokens: 600, Reasoning: &off})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Message.ToolCalls) == 0 {
		t.Fatalf("the model did not call the tool: %+v", first.Message)
	}
	call := first.Message.ToolCalls[0]
	var args struct {
		Path string `json:"path"`
	}
	if call.Name != "read_file" || json.Unmarshal([]byte(call.Arguments), &args) != nil || args.Path == "" {
		t.Fatalf("tool call = %+v", call)
	}
	msgs = append(msgs, first.Message, Message{Role: "tool", ToolCallID: call.ID, Content: "package main\n\nfunc main() {}\n"})
	second, err := tf.Complete(ctx, Request{Model: cfg.Models.Super, Messages: msgs, Tools: tools, MaxTokens: 600, Reasoning: &off})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second.Message.Content, "main") {
		t.Errorf("answer after the tool result = %q", second.Message.Content)
	}
	t.Logf("super: first %v, second %v", first.Latency.Round(time.Millisecond), second.Latency.Round(time.Millisecond))
}
