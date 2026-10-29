package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/gocomments"
	"github.com/aalvaropc/upkeep/internal/jsonfix"
	"github.com/aalvaropc/upkeep/internal/model"
)

const (
	resolveSystem = `You resolve merge conflicts in Go source files. Keep the intent of both sides whenever they can coexist: if both sides add different code at the same place, keep both. Choose one side only when the two cannot both be true, and then choose the side that matches its commit messages. Never add comments.

Answer with a single JSON object and nothing else:
{"files":[{"path":"<file>","resolutions":["<text that replaces the first conflict block>","<text for the second>"]}]}
Each resolution replaces only the lines between the markers, in the order the blocks appear in the file. Write those lines alone, without conflict markers and without repeating any line that appears above or below the markers.`

	diagnoseSystem = `You are a senior engineer. A pull request cannot be merged with its base branch because of merge conflicts that are too large to resolve automatically. In at most 120 words, in plain prose, say what each side was trying to do, why resolving it automatically would be unsafe, and how a person could split the work. Do not use headings or lists.`
)

const (
	maxConflictFileChars = 12000
	maxSubjects          = 12
)

type Resolution struct {
	Files    int
	Hunks    int
	Comments int
	Blocks   []ResolvedBlock
}

type ResolvedBlock struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

func (e Env) subjects(ctx context.Context, job *Job, rev string) string {
	subjects, err := e.Git.Subjects(ctx, job.Dir, rev, maxSubjects)
	if err != nil || len(subjects) == 0 {
		return "(none)"
	}
	return "- " + strings.Join(subjects, "\n- ")
}

func (e Env) conflictContext(ctx context.Context, job *Job, hunks []gitx.Hunk) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Commits on the pull request branch (marked ours):\n%s\n\nCommits on %s (marked theirs):\n%s\n", e.subjects(ctx, job, job.Base+"..HEAD"), job.Base, e.subjects(ctx, job, "HEAD.."+job.Base))
	seen := map[string]bool{}
	for _, h := range hunks {
		if seen[h.File] {
			continue
		}
		seen[h.File] = true
		data, err := os.ReadFile(filepath.Join(job.Dir, h.File))
		if err != nil {
			return "", err
		}
		text := string(data)
		if len(text) > maxConflictFileChars {
			text = text[:maxConflictFileChars] + "\n... the file continues ...\n"
		}
		fmt.Fprintf(&b, "\nFile %s with its conflict markers:\n```go\n%s\n```\n", h.File, strings.TrimRight(text, "\n"))
	}
	return b.String(), nil
}

type badAnswer struct{ err error }

func (b *badAnswer) Error() string { return b.err.Error() }

func (b *badAnswer) Unwrap() error { return b.err }

func bad(format string, args ...any) error {
	return &badAnswer{err: fmt.Errorf(format, args...)}
}

const resolveAttempts = 2

func (e Env) ResolveConflict(ctx context.Context, job *Job, hunks []gitx.Hunk) (Resolution, error) {
	user, err := e.conflictContext(ctx, job, hunks)
	if err != nil {
		return Resolution{}, err
	}
	var out Resolution
	var rejected error
	for attempt := 1; attempt <= resolveAttempts; attempt++ {
		prompt := user
		purpose := "resolve"
		if rejected != nil {
			prompt += fmt.Sprintf("\n\nYour previous answer was rejected: %v. Answer again with exactly one resolution per conflict block, in the JSON format given.", rejected)
			purpose = "resolve_retry"
		}
		out, err = e.tryResolve(ctx, job, hunks, purpose, prompt)
		if _, isBad := errors.AsType[*badAnswer](err); err == nil || !isBad {
			return out, err
		}
		rejected = err
		e.step(job, "conflict", "running", fmt.Sprintf("the answer was rejected (%v); asking again", err))
	}
	return out, rejected
}

func (e Env) tryResolve(ctx context.Context, job *Job, hunks []gitx.Hunk, purpose, user string) (Resolution, error) {
	var out Resolution
	answer, err := e.complete(ctx, job, model.Ultra, "conflict", purpose, resolveSystem, user, false, 4000)
	if err != nil {
		return out, err
	}
	plan, err := parseResolutions(answer)
	if err != nil {
		return out, &badAnswer{err: err}
	}
	perFile := map[string]int{}
	for _, h := range hunks {
		perFile[h.File]++
	}
	if len(plan) != len(perFile) {
		return out, bad("the model resolved %d files but %d have conflicts", len(plan), len(perFile))
	}
	type write struct {
		path string
		text string
	}
	var writes []write
	for file, count := range perFile {
		resolutions, ok := plan[file]
		if !ok {
			return out, bad("the model left %s unresolved", file)
		}
		if len(resolutions) != count {
			return out, bad("the model gave %d resolutions for the %d conflicts in %s", len(resolutions), count, file)
		}
		data, err := os.ReadFile(filepath.Join(job.Dir, file))
		if err != nil {
			return out, err
		}
		resolved, err := gitx.Resolve(string(data), resolutions)
		if err != nil {
			return out, bad("applying the resolutions to %s: %w", file, err)
		}
		if gitx.HasMarkers(resolved) {
			return out, bad("the resolution of %s still contains conflict markers", file)
		}
		writes = append(writes, write{path: file, text: resolved})
		out.Hunks += count
		if strings.HasSuffix(file, ".go") {
			out.Comments += gocomments.Added(data, []byte(resolved))
		}
		for _, r := range resolutions {
			out.Blocks = append(out.Blocks, ResolvedBlock{Path: file, Text: r})
		}
	}
	slices.SortStableFunc(out.Blocks, func(a, b ResolvedBlock) int { return strings.Compare(a.Path, b.Path) })
	for _, w := range writes {
		if err := os.WriteFile(filepath.Join(job.Dir, w.path), []byte(w.text), 0o600); err != nil {
			return out, err
		}
		out.Files++
	}
	if _, err := e.Runner.Format(ctx, job.Dir); err != nil {
		return out, err
	}
	return out, nil
}

func parseResolutions(answer string) (map[string][]string, error) {
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if start < 0 || end <= start {
		return nil, errors.New("the model did not answer with a JSON object")
	}
	var doc struct {
		Files []struct {
			Path        string   `json:"path"`
			Resolutions []string `json:"resolutions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(jsonfix.Repair(answer[start:end+1])), &doc); err != nil {
		return nil, fmt.Errorf("the model's answer is not valid JSON: %w", err)
	}
	plan := make(map[string][]string, len(doc.Files))
	for _, f := range doc.Files {
		clean := filepath.ToSlash(filepath.Clean(f.Path))
		if _, dup := plan[clean]; dup {
			return nil, fmt.Errorf("the model resolved %s twice", clean)
		}
		plan[clean] = f.Resolutions
	}
	return plan, nil
}

func (e Env) DiagnoseConflict(ctx context.Context, job *Job, hunks []gitx.Hunk) (string, error) {
	user, err := e.conflictContext(ctx, job, hunks)
	if err != nil {
		return "", err
	}
	answer, err := e.complete(ctx, job, model.Ultra, "diagnose", "diagnose", diagnoseSystem, user, true, 3000)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer), nil
}

func (e Env) DiagnoseFailure(ctx context.Context, job *Job, failure string, attempts int) (string, error) {
	const system = `You are a senior engineer. An automated agent tried to fix a Go pull request whose checks fail and could not. In at most 120 words, in plain prose, explain what the failure means, what is likely needed to fix it and why it needs a person. Do not use headings or lists.`
	user := fmt.Sprintf("The agent made %d attempts. The last failing step was:\n\n%s", attempts, strings.TrimSpace(failure))
	answer, err := e.complete(ctx, job, model.Ultra, "diagnose", "diagnose", system, user, true, 3000)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer), nil
}
