package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/aalvaropc/upkeep/internal/stub"
)

const DefaultTimeout = 120 * time.Second

type Spec struct {
	Dir     string
	Args    []string
	Network bool
	Timeout time.Duration
}

type Result struct {
	Args     []string      `json:"args"`
	ExitCode int           `json:"exit_code"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

func (r Result) OK() bool { return r.ExitCode == 0 && !r.TimedOut }

type Executor interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

type Exec struct{}

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
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now()
	err := cmd.Run()
	res := Result{Args: spec.Args, Output: out.String(), Duration: time.Since(start)}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	case err != nil:
		return res, err
	}
	return res, nil
}

type Docker struct {
	Image      string
	ModCache   string
	BuildCache string
	CPUs       string
	Memory     string
}

func (Docker) Run(_ context.Context, _ Spec) (Result, error) {
	return Result{}, stub.Err("docker sandbox")
}

type Report struct {
	Build  Result `json:"build"`
	Vet    Result `json:"vet"`
	Test   Result `json:"test"`
	Cached bool   `json:"cached"`
}

func (r Report) Passed() bool { return r.Build.OK() && r.Vet.OK() && r.Test.OK() }

type Runner interface {
	Download(ctx context.Context, dir string) (Result, error)
	Format(ctx context.Context, dir string) (Result, error)
	Check(ctx context.Context, dir string) (Report, error)
}

type GoRunner struct {
	Executor Executor
}

func (GoRunner) Download(_ context.Context, _ string) (Result, error) {
	return Result{}, stub.Err("download modules")
}

func (GoRunner) Format(_ context.Context, _ string) (Result, error) {
	return Result{}, stub.Err("format code")
}

func (GoRunner) Check(_ context.Context, _ string) (Report, error) {
	return Report{}, stub.Err("build, vet and test")
}
