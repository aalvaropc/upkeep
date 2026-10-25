package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{name: "version", args: []string{"version"}, code: 0, stdout: "upkeep "},
		{name: "version flag", args: []string{"--version"}, code: 0, stdout: "upkeep "},
		{name: "help", args: []string{"help"}, code: 0, stdout: "usage: upkeep"},
		{name: "no command", args: nil, code: 2, stderr: "usage: upkeep"},
		{name: "unknown command", args: []string{"nope"}, code: 2, stderr: `unknown command "nope"`},
		{name: "run", args: []string{"run"}, code: 1, stderr: "run: not implemented"},
		{name: "demo", args: []string{"demo"}, code: 1, stderr: "demo: not implemented"},
		{name: "record", args: []string{"record"}, code: 1, stderr: "record: not implemented"},
		{name: "eval", args: []string{"eval"}, code: 1, stderr: "eval: not implemented"},
		{name: "serve with arguments", args: []string{"serve", "extra"}, code: 1, stderr: "serve takes no arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(t.Context(), tt.args, &stdout, &stderr, nil)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tt.code, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.stdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.stdout)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.stderr)
			}
		})
	}
}

func TestServeRejectsInvalidConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{"serve"}, &stdout, &stderr, []string{"UPKEEP_MODE=fast"})
	if code != 1 || !strings.Contains(stderr.String(), "UPKEEP_MODE") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

type signalWriter chan string

func (w signalWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

func TestServe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stdout := make(signalWriter, 1)
	environ := []string{"UPKEEP_ADDR=127.0.0.1:0", "UPKEEP_DATA_DIR=" + t.TempDir()}
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"serve"}, stdout, io.Discard, environ) }()

	var url string
	select {
	case line := <-stdout:
		var ok bool
		if url, ok = strings.CutPrefix(strings.TrimSpace(line), "upkeep listening on "); !ok {
			t.Fatalf("stdout = %q", line)
		}
	case code := <-done:
		t.Fatalf("serve exited early with %d", code)
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not start")
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("serve exited with %d after cancel, want 0", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop after cancel")
	}
}
