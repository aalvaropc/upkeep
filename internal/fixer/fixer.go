package fixer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

const (
	DefaultMaxTurns  = 12
	maxPromptFiles   = 6
	maxPromptFileLen = 12000
	maxContextChars  = 40000
)

const SystemPrompt = `You repair a Go repository so that go build, go vet and go test pass again after a dependency upgrade or after the base branch changed.

Rules:
- Never edit test files; the tools reject it.
- Never add comments of any kind; the tools reject it.
- Make the smallest change that works and touch only what is needed.
- Look up an API with go_doc instead of guessing it. Do not invent packages.
- Use apply_edit to change code; old must match the file exactly once.
- Call run_checks after your edits. When it reports PASS you are done.`

type Stop string

const (
	StopPassed   Stop = "checks passed"
	StopTurns    Stop = "turn limit"
	StopTokens   Stop = "token budget"
	StopDeadline Stop = "time limit"
	StopQuit     Stop = "the model stopped without passing the checks"
	StopModel    Stop = "the model call failed"
)

type Call struct {
	Model   string        `json:"model"`
	Latency time.Duration `json:"latency"`
	Usage   model.Usage   `json:"usage"`
	Charge  model.Charge  `json:"charge"`
}

type Attempt struct {
	Success bool         `json:"success"`
	Stop    Stop         `json:"stop"`
	Turns   int          `json:"turns"`
	Calls   []Call       `json:"calls"`
	Usage   model.Usage  `json:"usage"`
	Charge  model.Charge `json:"charge"`
	Edits   []Edit       `json:"edits"`
	Checks  int          `json:"checks"`
	Failure string       `json:"failure,omitempty"`
	Report  sandbox.Report
	Err     error `json:"-"`
}

type Fixer struct {
	Model     model.Model
	ModelName string
	Meter     model.Meter
	Runner    sandbox.Runner
	Executor  sandbox.Executor
	Emit      func(kind events.Kind, data any)
	MaxTurns  int
	MaxTokens int
	Deadline  time.Duration
}

type Task struct {
	Dir     string
	Failure string
	Notes   string
	Prior   string
}

func (f Fixer) emit(kind events.Kind, data any) {
	if f.Emit != nil {
		f.Emit(kind, data)
	}
}

func (f Fixer) Run(ctx context.Context, task Task) Attempt {
	var a Attempt
	if f.Model == nil || f.Runner == nil || f.Executor == nil {
		a.Err = errors.New("a fixer needs a model, a runner and an executor")
		return a
	}
	box, err := newToolbox(task.Dir, f.Runner, f.Executor)
	if err != nil {
		a.Err = err
		return a
	}
	defer box.close()

	maxTurns := f.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}
	if f.Deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.Deadline)
		defer cancel()
	}
	off := false
	msgs := []model.Message{
		{Role: "system", Content: SystemPrompt},
		{Role: "user", Content: BuildPrompt(task)},
	}
	nudged := false

	for a.Turns < maxTurns {
		if f.MaxTokens > 0 && a.Usage.Total() >= f.MaxTokens {
			a.Stop = StopTokens
			break
		}
		resp, err := f.Model.Complete(ctx, model.Request{
			Model:     f.ModelName,
			Messages:  msgs,
			Tools:     Tools(),
			MaxTokens: 6000,
			Reasoning: &off,
		})
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				a.Stop = StopDeadline
			} else {
				a.Stop = StopModel
				a.Err = err
			}
			break
		}
		a.Turns++
		charge := f.Meter.Charge(f.ModelName, resp.Usage)
		a.Calls = append(a.Calls, Call{Model: f.ModelName, Latency: resp.Latency, Usage: resp.Usage, Charge: charge})
		a.Usage = a.Usage.Add(resp.Usage)
		a.Charge = a.Charge.Add(charge)
		f.emit(events.ModelCall, map[string]any{
			"model": f.ModelName, "latency_ms": resp.Latency.Milliseconds(), "prompt": resp.Usage.PromptTokens,
			"completion": resp.Usage.CompletionTokens, "reasoning": resp.Usage.ReasoningTokens, "micros": charge.Micros,
		})

		reply := resp.Message
		reply.Reasoning = ""
		msgs = append(msgs, reply)
		if len(reply.ToolCalls) == 0 {
			if box.passed {
				break
			}
			if nudged {
				a.Stop = StopQuit
				break
			}
			nudged = true
			msgs = append(msgs, model.Message{Role: "user", Content: "The checks have not passed yet. Call run_checks, or make an edit and then call run_checks."})
			continue
		}
		for _, tc := range reply.ToolCalls {
			res := box.call(ctx, tc)
			f.emit(events.ToolCall, map[string]any{"tool": tc.Name, "ok": res.ok, "detail": detail(tc, res)})
			msgs = append(msgs, model.Message{Role: "tool", ToolCallID: tc.ID, Content: res.text})
		}
		if box.passed {
			a.Stop = StopPassed
			break
		}
	}

	if a.Stop == "" {
		switch {
		case box.passed:
			a.Stop = StopPassed
		case a.Turns >= maxTurns:
			a.Stop = StopTurns
		default:
			a.Stop = StopQuit
		}
	}
	a.Success = box.passed && a.Stop == StopPassed
	a.Edits = box.edits
	a.Checks = box.checks
	a.Failure = box.failure
	a.Report = box.last
	return a
}

func detail(tc model.ToolCall, res toolResult) string {
	if !res.ok {
		return firstLine(strings.TrimPrefix(res.text, "error: "))
	}
	switch tc.Name {
	case "run_checks":
		return firstLine(res.text)
	default:
		return tc.Name
	}
}

var mention = regexp.MustCompile(`([A-Za-z0-9_./-]+\.go):\d+`)

func MentionedFiles(dir, output string) []string {
	var files []string
	seen := map[string]bool{}
	for _, m := range mention.FindAllStringSubmatch(output, -1) {
		rel := filepath.ToSlash(filepath.Clean(m[1]))
		if seen[rel] || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if _, err := readablePath(rel); err != nil {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, rel))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		seen[rel] = true
		files = append(files, rel)
		if len(files) == maxPromptFiles {
			break
		}
	}
	return files
}

func BuildPrompt(task Task) string {
	var b strings.Builder
	b.WriteString("The checks fail. Output of the first failing step:\n\n")
	b.WriteString(strings.TrimSpace(task.Failure))
	total := b.Len()
	for _, rel := range MentionedFiles(task.Dir, task.Failure) {
		data, err := os.ReadFile(filepath.Join(task.Dir, rel))
		if err != nil {
			continue
		}
		text := string(data)
		if len(text) > maxPromptFileLen {
			text = text[:maxPromptFileLen] + "\n... the file continues ...\n"
		}
		if total+len(text) > maxContextChars {
			break
		}
		total += len(text)
		fmt.Fprintf(&b, "\n\nContents of %s:\n```go\n%s\n```", rel, strings.TrimRight(text, "\n"))
	}
	if notes := strings.TrimSpace(task.Notes); notes != "" {
		fmt.Fprintf(&b, "\n\nRelease notes and migration guidance for the upgrade (fetched from the web, may be partly irrelevant):\n\n%s", notes)
	}
	if prior := strings.TrimSpace(task.Prior); prior != "" {
		fmt.Fprintf(&b, "\n\nA previous attempt did not work. Its last failure was:\n\n%s", prior)
	}
	b.WriteString("\n\nFix the repository.")
	return b.String()
}
