package strategy

import (
	"context"
	"fmt"
	"strings"
)

func (e Env) CommitMerge(ctx context.Context, job *Job) error {
	msg := fmt.Sprintf("merge %s into %s", strings.TrimPrefix(job.Base, "origin/"), job.PR.Branch)
	if _, err := e.Git.Commit(ctx, job.Dir, msg); err != nil {
		return fmt.Errorf("committing the merge: %w", err)
	}
	return nil
}

func (e Env) Format(ctx context.Context, job *Job) ([]string, error) {
	res, err := e.Runner.Format(ctx, job.Dir)
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, fmt.Errorf("formatting failed: %s", firstLine(res.Output))
	}
	files := strings.Fields(res.Output)
	if len(files) == 0 {
		return nil, nil
	}
	if _, err := e.Git.Commit(ctx, job.Dir, "style: format the Go files with gofmt"); err != nil {
		return nil, fmt.Errorf("committing the formatting: %w", err)
	}
	return files, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
