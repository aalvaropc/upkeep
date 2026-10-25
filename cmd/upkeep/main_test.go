package main

import (
	"bytes"
	"strings"
	"testing"
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
