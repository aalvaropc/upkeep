package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aalvaropc/upkeep/internal/fixture"
)

const ReportKind = "report"

type Report struct {
	Build  Result `json:"build"`
	Vet    Result `json:"vet"`
	Test   Result `json:"test"`
	Cached bool   `json:"cached"`
}

func (r Report) Passed() bool { return r.Build.OK() && r.Vet.OK() && r.Test.OK() }

func (r Report) FirstFailure() (Result, bool) {
	for _, res := range []Result{r.Build, r.Vet, r.Test} {
		if !res.OK() && !res.Skipped {
			return res, true
		}
	}
	return Result{}, false
}

type Runner interface {
	Download(ctx context.Context, dir string) (Result, error)
	Format(ctx context.Context, dir string) (Result, error)
	Unformatted(ctx context.Context, dir string) ([]string, error)
	Check(ctx context.Context, dir string) (Report, error)
	Tests(ctx context.Context, dir, pattern string) (Result, error)
}

type GoRunner struct {
	Executor Executor
	Cache    fixture.Store
	Variant  string
}

func (r GoRunner) run(ctx context.Context, dir string, network bool, args ...string) (Result, error) {
	if r.Executor == nil {
		return Result{}, errors.New("sandbox: no executor")
	}
	return r.Executor.Run(ctx, Spec{Dir: dir, Args: args, Network: network})
}

func (r GoRunner) Download(ctx context.Context, dir string) (Result, error) {
	return r.run(ctx, dir, true, "go", "mod", "download")
}

func (r GoRunner) Format(ctx context.Context, dir string) (Result, error) {
	listed, err := r.run(ctx, dir, false, "gofmt", "-l", ".")
	if err != nil || !listed.OK() {
		return listed, err
	}
	if strings.TrimSpace(listed.Output) == "" {
		return listed, nil
	}
	written, err := r.run(ctx, dir, false, "gofmt", "-w", ".")
	if err != nil || !written.OK() {
		return written, err
	}
	written.Output = listed.Output
	return written, nil
}

func (r GoRunner) Unformatted(ctx context.Context, dir string) ([]string, error) {
	res, err := r.run(ctx, dir, false, "gofmt", "-l", ".")
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, fmt.Errorf("gofmt could not read the tree: %s", firstLine(res.Output))
	}
	return strings.Fields(res.Output), nil
}

func (r GoRunner) Tests(ctx context.Context, dir, pattern string) (Result, error) {
	return r.run(ctx, dir, false, "go", "test", "-count=1", "-run", pattern, "./...")
}

func skipped(args ...string) Result { return Result{Args: args, Skipped: true, ExitCode: -1} }

func (r GoRunner) Check(ctx context.Context, dir string) (Report, error) {
	key := ""
	if r.Cache != nil {
		var err error
		if key, err = TreeKey(dir, r.Variant); err != nil {
			return Report{}, err
		}
		var cached Report
		if err := r.Cache.Load(ReportKind, key, &cached); err == nil {
			cached.Cached = true
			return cached, nil
		} else if !errors.Is(err, fixture.ErrMiss) {
			return Report{}, err
		}
	}
	report, err := r.check(ctx, dir)
	if err != nil {
		return report, err
	}
	if r.Cache != nil && !timedOut(report) {
		if err := r.Cache.Save(ReportKind, key, report); err != nil {
			return report, fmt.Errorf("saving the report: %w", err)
		}
	}
	return report, nil
}

func timedOut(r Report) bool { return r.Build.TimedOut || r.Vet.TimedOut || r.Test.TimedOut }

func (r GoRunner) check(ctx context.Context, dir string) (Report, error) {
	report := Report{
		Vet:  skipped("go", "vet", "./..."),
		Test: skipped("go", "test", "-count=1", "./..."),
	}
	var err error
	if report.Build, err = r.run(ctx, dir, false, "go", "build", "./..."); err != nil || !report.Build.OK() {
		return report, err
	}
	if report.Vet, err = r.run(ctx, dir, false, "go", "vet", "./..."); err != nil || !report.Vet.OK() {
		return report, err
	}
	report.Test, err = r.run(ctx, dir, false, "go", "test", "-count=1", "./...")
	return report, err
}

func TreeKey(dir, variant string) (string, error) {
	h := sha256.New()
	fmt.Fprint(h, variant)
	h.Write([]byte{0})
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%o\x00", filepath.ToSlash(rel), info.Mode().Perm())
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "link:%s\x00", target)
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		h.Write([]byte{0})
		return err
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
