package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	DefaultTimeout = 120 * time.Second
	headLimit      = 96 << 10
	tailLimit      = 32 << 10
	truncationNote = "\n... output truncated ...\n"
)

type Spec struct {
	Dir     string
	Args    []string
	Env     []string
	Network bool
	Timeout time.Duration
}

type Result struct {
	Args      []string      `json:"args"`
	ExitCode  int           `json:"exit_code"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	TimedOut  bool          `json:"timed_out"`
	Skipped   bool          `json:"skipped"`
	Truncated bool          `json:"truncated"`
}

func (r Result) OK() bool { return r.ExitCode == 0 && !r.TimedOut && !r.Skipped }

type Executor interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

type Exec struct{}

var secretName = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL"}

func CleanEnviron(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		secret := false
		for _, marker := range secretName {
			if strings.Contains(upper, marker) {
				secret = true
				break
			}
		}
		if !secret {
			out = append(out, kv)
		}
	}
	return out
}

func (Exec) Run(ctx context.Context, spec Spec) (Result, error) {
	if len(spec.Args) == 0 {
		return Result{}, errors.New("sandbox: no command")
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(CleanEnviron(os.Environ()), spec.Env...)
	out := newBoundedBuffer()
	cmd.Stdout = out
	cmd.Stderr = out
	start := time.Now()
	err := cmd.Run()
	return finish(ctx, spec, out, time.Since(start), err)
}

func finish(ctx context.Context, spec Spec, out *boundedBuffer, took time.Duration, err error) (Result, error) {
	res := Result{Args: spec.Args, Output: out.String(), Duration: took, Truncated: out.truncated()}
	if err == nil {
		return res, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

type boundedBuffer struct {
	head  bytes.Buffer
	ring  []byte
	next  int
	count int
	total int64
}

func newBoundedBuffer() *boundedBuffer {
	return &boundedBuffer{ring: make([]byte, tailLimit)}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	b.total += int64(n)
	if room := headLimit - b.head.Len(); room > 0 {
		take := min(room, len(p))
		b.head.Write(p[:take])
		p = p[take:]
	}
	for len(p) > 0 {
		copied := copy(b.ring[b.next:], p)
		p = p[copied:]
		b.next = (b.next + copied) % tailLimit
		b.count = min(b.count+copied, tailLimit)
	}
	return n, nil
}

func (b *boundedBuffer) truncated() bool { return b.total > headLimit+tailLimit }

func (b *boundedBuffer) tailBytes() []byte {
	if b.count < tailLimit {
		return b.ring[:b.count]
	}
	return append(append([]byte{}, b.ring[b.next:]...), b.ring[:b.next]...)
}

func (b *boundedBuffer) String() string {
	var sb strings.Builder
	sb.WriteString(b.head.String())
	if b.truncated() {
		sb.WriteString(truncationNote)
	}
	sb.Write(b.tailBytes())
	return sb.String()
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}
