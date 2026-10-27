package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/eval"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		environ []string
		code    int
		stdout  string
		stderr  string
	}{
		{name: "version", args: []string{"version"}, code: 0, stdout: "upkeep "},
		{name: "version flag", args: []string{"--version"}, code: 0, stdout: "upkeep "},
		{name: "help", args: []string{"help"}, code: 0, stdout: "usage: upkeep"},
		{name: "no command", args: nil, code: 2, stderr: "usage: upkeep"},
		{name: "unknown command", args: []string{"nope"}, code: 2, stderr: `unknown command "nope"`},
		{name: "run without a repository", args: []string{"run"}, code: 1, stderr: "run: -repo owner/name is required"},
		{name: "run with a bad repository", args: []string{"run", "-repo", "a/b/c"}, code: 1, stderr: "run: -repo owner/name is required"},
		{name: "run without a model key", args: []string{"run", "-repo", "o/r"}, code: 1, stderr: "needs NEBIUS_API_KEY"},
		{name: "run without a token", args: []string{"run", "-repo", "o/r"}, environ: []string{"NEBIUS_API_KEY=k"}, code: 1, stderr: "GITHUB_TOKEN is required"},
		{name: "run with arguments", args: []string{"run", "-repo", "o/r", "extra"}, code: 1, stderr: "run takes no arguments"},
		{name: "run with a bad pull request number", args: []string{"run", "-repo", "o/r", "-pr", "9,x"}, code: 1, stderr: `"x" is not a pull request number`},
		{name: "run with no parallelism", args: []string{"run", "-repo", "o/r", "-parallel", "0"}, code: 1, stderr: "-parallel must be at least 1"},
		{name: "demo with arguments", args: []string{"demo", "extra"}, code: 1, stderr: "demo takes no arguments"},
		{name: "demo with a bad flag", args: []string{"demo", "-pace", "-1"}, code: 1, stderr: "-pace cannot be negative"},
		{name: "record without a directory", args: []string{"record"}, code: 1, stderr: "record: -fixtures DIR is required"},
		{name: "record with arguments", args: []string{"record", "-fixtures", "x", "extra"}, code: 1, stderr: "record takes no arguments"},
		{name: "eval with a bad run count", args: []string{"eval", "-runs", "0"}, code: 1, stderr: "-runs must be at least 1"},
		{name: "eval with arguments", args: []string{"eval", "extra"}, code: 1, stderr: "eval takes no arguments"},
		{name: "scenario without a subcommand", args: []string{"scenario"}, code: 1, stderr: "usage: upkeep scenario build"},
		{name: "scenario build without a directory", args: []string{"scenario", "build"}, code: 1, stderr: "-out is required"},
		{name: "scenario build from a missing repository", args: []string{"scenario", "build", "-out", t.TempDir() + "/forge.git", "-source", t.TempDir() + "/missing"}, code: 1, stderr: "upkeep:"},
		{name: "serve with arguments", args: []string{"serve", "extra"}, code: 1, stderr: "serve takes no arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(t.Context(), tt.args, &stdout, &stderr, tt.environ)
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

func TestWriteReportKeepsEachRunAndTheLatestSummary(t *testing.T) {
	dir := t.TempDir()
	report := eval.Report{Scenario: "tiny", Mode: "replay", Version: "dev", Runs: 1, At: time.Date(2026, 10, 25, 14, 5, 9, 0, time.UTC), Cases: []eval.Case{{Run: 1, Number: 1, Title: "Change a", OK: true}}}
	if err := writeReport(dir, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "results", "20261025T140509Z-replay.json"))
	if err != nil || !strings.Contains(string(data), `"scenario": "tiny"`) {
		t.Fatalf("results file: %v %s", err, data)
	}
	md, err := os.ReadFile(filepath.Join(dir, "latest.md"))
	if err != nil || !strings.Contains(string(md), "1 of 1 cases ended as expected") {
		t.Fatalf("latest.md: %v %s", err, md)
	}
}
