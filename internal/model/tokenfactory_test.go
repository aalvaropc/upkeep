package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
)

var testKey = config.Secret(strings.Repeat("k", 40))

const okBody = `{
  "choices": [{"finish_reason": "stop", "message": {"role": "assistant", "content": "hello", "reasoning_content": "thinking"}}],
  "usage": {"prompt_tokens": 100, "completion_tokens": 40, "completion_tokens_details": {"reasoning_tokens": 25}, "prompt_tokens_details": {"cached_tokens": 60}}
}`

func noWait(int, time.Duration) time.Duration { return 0 }

func client(srv *httptest.Server) *TokenFactory {
	tf := NewTokenFactory(srv.URL+"/v1/", testKey)
	tf.HTTP = srv.Client()
	tf.Backoff = noWait
	return tf
}

func TestCompleteParsesTheResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, okBody)
	}))
	defer srv.Close()
	resp, err := client(srv).Complete(t.Context(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "hello" || resp.Message.Reasoning != "thinking" || resp.Message.Role != "assistant" || resp.FinishReason != "stop" {
		t.Fatalf("message = %+v finish %q", resp.Message, resp.FinishReason)
	}
	want := Usage{PromptTokens: 100, CompletionTokens: 40, ReasoningTokens: 25, CachedPromptTokens: 60}
	if resp.Usage != want {
		t.Fatalf("usage = %+v, want %+v", resp.Usage, want)
	}
	if resp.Latency <= 0 {
		t.Fatalf("latency = %v", resp.Latency)
	}
}

func TestRequestShape(t *testing.T) {
	var path, auth, ctype string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth, ctype = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, okBody)
	}))
	defer srv.Close()

	off := false
	temp := 0.2
	_, err := client(srv).Complete(t.Context(), Request{
		Model:       "nvidia/some-model",
		MaxTokens:   900,
		Temperature: &temp,
		Reasoning:   &off,
		Messages: []Message{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "fix it"},
			{Role: "assistant", Reasoning: "never sent back", ToolCalls: []ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`}}},
			{Role: "tool", ToolCallID: "c1", Content: "package a"},
		},
		Tools: []Tool{
			{Name: "read_file", Description: "Read a file.", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
			{Name: "run_checks", Description: "Run the checks."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" || ctype != "application/json" || auth != "Bearer "+testKey.Reveal() {
		t.Fatalf("path %q, content type %q, auth header set: %v", path, ctype, auth == "Bearer "+testKey.Reveal())
	}
	if body["model"] != "nvidia/some-model" || body["max_tokens"] != float64(900) || body["temperature"] != 0.2 {
		t.Fatalf("body = %v", body)
	}
	kwargs, _ := body["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false {
		t.Fatalf("chat_template_kwargs = %v, want enable_thinking false", body["chat_template_kwargs"])
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("messages = %v", msgs)
	}
	assistant, _ := msgs[2].(map[string]any)
	if assistant["content"] != nil {
		t.Errorf("an assistant message with only tool calls should send null content, got %v", assistant["content"])
	}
	if _, leaked := assistant["reasoning_content"]; leaked {
		t.Error("the reasoning of an earlier turn was sent back to the model")
	}
	calls, _ := assistant["tool_calls"].([]any)
	call, _ := calls[0].(map[string]any)
	fn, _ := call["function"].(map[string]any)
	if call["id"] != "c1" || call["type"] != "function" || fn["name"] != "read_file" || fn["arguments"] != `{"path":"a.go"}` {
		t.Errorf("tool call = %v", call)
	}
	tool, _ := msgs[3].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "c1" || tool["content"] != "package a" {
		t.Errorf("tool message = %v", tool)
	}
	tools, _ := body["tools"].([]any)
	second, _ := tools[1].(map[string]any)
	params, _ := second["function"].(map[string]any)["parameters"].(map[string]any)
	if len(tools) != 2 || params["type"] != "object" {
		t.Errorf("tools = %v; a tool without parameters should get an empty object schema", tools)
	}
}

func TestReasoningIsOnlySentWhenDisabled(t *testing.T) {
	for name, reasoning := range map[string]*bool{"unset": nil, "on": new(true)} {
		var body map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&body)
			fmt.Fprint(w, okBody)
		}))
		if _, err := client(srv).Complete(t.Context(), Request{Model: "m", Reasoning: reasoning, Messages: []Message{{Role: "user", Content: "x"}}}); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		if _, present := body["chat_template_kwargs"]; present {
			t.Errorf("%s: chat_template_kwargs was sent", name)
		}
	}
}

func TestToolCallsInTheResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[
			{"id":"a1","type":"function","function":{"name":"apply_edit","arguments":"{\"path\":\"x.go\"}"}},
			{"type":"function","function":{"name":"run_checks","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer srv.Close()
	resp, err := client(srv).Complete(t.Context(), Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	calls := resp.Message.ToolCalls
	if len(calls) != 2 || calls[0].ID != "a1" || calls[0].Name != "apply_edit" || calls[0].Arguments != `{"path":"x.go"}` {
		t.Fatalf("tool calls = %+v", calls)
	}
	if calls[1].ID != "call_1" || calls[1].Name != "run_checks" {
		t.Fatalf("a call without an id should get a stable one: %+v", calls[1])
	}
	if resp.Message.Content != "" || resp.FinishReason != "tool_calls" {
		t.Fatalf("content %q finish %q", resp.Message.Content, resp.FinishReason)
	}
}

func TestRetriesRateLimitsAndServerErrors(t *testing.T) {
	var hits atomic.Int32
	var waits []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch hits.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"message":"slow down"}}`)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			fmt.Fprint(w, okBody)
		}
	}))
	defer srv.Close()
	tf := client(srv)
	tf.Backoff = func(attempt int, retryAfter time.Duration) time.Duration {
		waits = append(waits, retryAfter)
		return 0
	}
	if _, err := tf.Complete(t.Context(), Request{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 3", hits.Load())
	}
	if len(waits) != 2 || waits[0] != 2*time.Second || waits[1] != 0 {
		t.Fatalf("backoff saw retry-after values %v, want [2s 0s]", waits)
	}
}

func TestGivesUpAfterTheRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"message":"overloaded"}}`)
	}))
	defer srv.Close()
	tf := client(srv)
	tf.MaxRetries = 2
	_, err := tf.Complete(t.Context(), Request{Model: "m"})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Status != 503 || apiErr.Message != "overloaded" {
		t.Fatalf("error = %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 1 try and 2 retries", hits.Load())
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"detail":"max_tokens is too large"}`)
	}))
	defer srv.Close()
	_, err := client(srv).Complete(t.Context(), Request{Model: "m"})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Status != 400 || apiErr.Message != "max_tokens is too large" || apiErr.Retryable() {
		t.Fatalf("error = %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, a 400 must not be retried", hits.Load())
	}
}

func TestCancellationStopsTheBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	tf := client(srv)
	tf.Backoff = func(int, time.Duration) time.Duration { return time.Hour }
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := tf.Complete(ctx, Request{Model: "m"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want the context deadline", err)
	}
	if _, ok := errors.AsType[*APIError](err); !ok {
		t.Fatalf("the last API error should be kept: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("the call did not stop when the context ended")
	}
}

func TestTransportErrorsAreRetriedAndNeverShowTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()
	tf := client(srv)
	tf.MaxRetries = 1
	_, err := tf.Complete(t.Context(), Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "token factory request failed") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), testKey.Reveal()) {
		t.Fatal("the error mentions the key")
	}
}

func TestSanitizeRemovesTheKey(t *testing.T) {
	err := sanitize(errors.New("Post https://x?key="+testKey.Reveal()+": boom"), testKey)
	if strings.Contains(err.Error(), testKey.Reveal()) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("sanitized = %v", err)
	}
	plain := errors.New("boom")
	if !errors.Is(sanitize(plain, testKey), plain) {
		t.Fatal("an error without the key should be returned unchanged")
	}
}

func TestBadResponses(t *testing.T) {
	for name, body := range map[string]string{
		"not json":         `<html>`,
		"no choices":       `{"choices":[],"usage":{}}`,
		"nameless call":    `{"choices":[{"message":{"tool_calls":[{"id":"x","function":{"arguments":"{}"}}]}}]}`,
		"empty object":     `{}`,
		"null choice list": `{"choices":null}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := client(srv).Complete(t.Context(), Request{Model: "m"})
		srv.Close()
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := (&TokenFactory{}).Complete(t.Context(), Request{}); err == nil {
		t.Error("a request without a model was accepted")
	}
}

func TestRetryAfterParsing(t *testing.T) {
	if got := parseRetryAfter("1.5"); got != 1500*time.Millisecond {
		t.Errorf("seconds: %v", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty: %v", got)
	}
	if got := parseRetryAfter("soon"); got != 0 {
		t.Errorf("garbage: %v", got)
	}
	future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got < 5*time.Second || got > 11*time.Second {
		t.Errorf("http date: %v", got)
	}
}

func TestDefaultBackoff(t *testing.T) {
	if got := DefaultBackoff(0, 7*time.Second); got != 7*time.Second {
		t.Errorf("retry-after should win: %v", got)
	}
	if got := DefaultBackoff(0, time.Hour); got != 30*time.Second {
		t.Errorf("retry-after should be capped: %v", got)
	}
	for attempt, floor := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		got := DefaultBackoff(attempt, 0)
		if got < floor || got > floor+floor/4 {
			t.Errorf("attempt %d: %v outside [%v, %v]", attempt, got, floor, floor+floor/4)
		}
	}
	if got := DefaultBackoff(20, 0); got != 30*time.Second {
		t.Errorf("the wait should be capped: %v", got)
	}
}
