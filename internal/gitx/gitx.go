package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultTimeout = 2 * time.Minute

var (
	ErrNotAncestor = errors.New("not an ancestor")
	ErrForcePush   = errors.New("refusing to push a forced or deleting ref")
)

type Git struct {
	Bin         string
	AuthorName  string
	AuthorEmail string
	Timeout     time.Duration
}

type Hunk struct {
	File   string `json:"file"`
	Start  int    `json:"start"`
	Lines  int    `json:"lines"`
	Ours   string `json:"ours"`
	Theirs string `json:"theirs"`
}

type MergeResult struct {
	Clean     bool   `json:"clean"`
	Conflicts []Hunk `json:"conflicts,omitempty"`
}

func (r MergeResult) Hunks() int { return len(r.Conflicts) }

func (r MergeResult) LargestHunk() int {
	most := 0
	for _, h := range r.Conflicts {
		most = max(most, h.Lines)
	}
	return most
}

type FileChange struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

func (c FileChange) IsTest() bool {
	return strings.HasSuffix(c.Path, "_test.go") || strings.HasPrefix(c.Path, "testdata/") || strings.Contains(c.Path, "/testdata/")
}

type Error struct {
	Args   []string
	Output string
	Code   int
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", strings.Join(e.Args, " "), e.Code, strings.TrimSpace(e.Output))
}

func (g Git) bin() string {
	if g.Bin != "" {
		return g.Bin
	}
	return "git"
}

func (g Git) identity() []string {
	name, email := g.AuthorName, g.AuthorEmail
	if name == "" {
		name = "Upkeep"
	}
	if email == "" {
		email = "upkeep@users.noreply.github.com"
	}
	return []string{"-c", "user.name=" + name, "-c", "user.email=" + email, "-c", "commit.gpgsign=false"}
}

func (g Git) run(ctx context.Context, dir string, stdin string, args ...string) (string, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	full := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "advice.detachedHead=false"}, g.identity()...)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, g.bin(), full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1")
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return out.String(), fmt.Errorf("git %s: %w", strings.Join(args, " "), ctx.Err())
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return out.String(), &Error{Args: args, Output: out.String(), Code: exitErr.ExitCode()}
		}
		return out.String(), err
	}
	return out.String(), nil
}

func (g Git) Clone(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	_, err := g.run(ctx, "", "", "clone", "--quiet", "--", src, dst)
	return err
}

func (g Git) Fetch(ctx context.Context, dir, remote string, refspecs ...string) error {
	args := append([]string{"fetch", "--quiet", remote}, refspecs...)
	_, err := g.run(ctx, dir, "", args...)
	return err
}

func (g Git) Checkout(ctx context.Context, dir, ref string, create bool) error {
	args := []string{"checkout", "--quiet"}
	if create {
		args = append(args, "-B")
	}
	args = append(args, ref)
	if !create {
		args = append(args, "--")
	}
	_, err := g.run(ctx, dir, "", args...)
	return err
}

func (g Git) Head(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "", "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

func (g Git) RevParse(ctx context.Context, dir, ref string) (string, error) {
	out, err := g.run(ctx, dir, "", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return strings.TrimSpace(out), err
}

func (g Git) IsAncestor(ctx context.Context, dir, ancestor, descendant string) (bool, error) {
	_, err := g.run(ctx, dir, "", "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	if gitErr, ok := errors.AsType[*Error](err); ok && gitErr.Code == 1 {
		return false, nil
	}
	return false, err
}

func (g Git) Merge(ctx context.Context, dir, ref string) (MergeResult, error) {
	_, err := g.run(ctx, dir, "", "merge", "--no-commit", "--no-ff", ref)
	if err == nil {
		return MergeResult{Clean: true}, nil
	}
	gitErr, ok := errors.AsType[*Error](err)
	if !ok || gitErr.Code != 1 {
		return MergeResult{}, err
	}
	files, listErr := g.unmerged(ctx, dir)
	if listErr != nil {
		return MergeResult{}, listErr
	}
	if len(files) == 0 {
		return MergeResult{}, err
	}
	var result MergeResult
	for _, file := range files {
		data, readErr := os.ReadFile(filepath.Join(dir, file))
		if readErr != nil {
			return MergeResult{}, readErr
		}
		hunks, parseErr := ParseConflicts(file, string(data))
		if parseErr != nil {
			return MergeResult{}, parseErr
		}
		result.Conflicts = append(result.Conflicts, hunks...)
	}
	return result, nil
}

func (g Git) unmerged(ctx context.Context, dir string) ([]string, error) {
	out, err := g.run(ctx, dir, "", "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

func (g Git) MergeInProgress(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git", "MERGE_HEAD"))
	return err == nil
}

func (g Git) Subjects(ctx context.Context, dir, rev string, limit int) ([]string, error) {
	if strings.HasPrefix(rev, "-") {
		return nil, fmt.Errorf("invalid revision %q", rev)
	}
	out, err := g.run(ctx, dir, "", "log", "--format=%s", "-n", strconv.Itoa(max(limit, 1)), rev, "--")
	if err != nil {
		return nil, err
	}
	var subjects []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			subjects = append(subjects, line)
		}
	}
	return subjects, nil
}

func (g Git) MergeAbort(ctx context.Context, dir string) error {
	_, err := g.run(ctx, dir, "", "merge", "--abort")
	return err
}

func (g Git) Diff(ctx context.Context, dir, base string) ([]FileChange, error) {
	if _, err := g.run(ctx, dir, "", "add", "-A"); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, dir, "", "diff", "--cached", "--numstat", "-z", base, "--")
	if err != nil {
		return nil, err
	}
	return parseNumstat(out)
}

func parseNumstat(out string) ([]FileChange, error) {
	var changes []FileChange
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		row := fields[i]
		if row == "" {
			continue
		}
		parts := strings.SplitN(row, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected numstat row %q", row)
		}
		path := parts[2]
		if path == "" && i+2 < len(fields) {
			path = fields[i+2]
			i += 2
		}
		c := FileChange{Path: path}
		if parts[0] == "-" || parts[1] == "-" {
			c.Added, c.Deleted = 1, 1
		} else {
			var err error
			if c.Added, err = strconv.Atoi(parts[0]); err != nil {
				return nil, fmt.Errorf("unexpected numstat row %q", row)
			}
			if c.Deleted, err = strconv.Atoi(parts[1]); err != nil {
				return nil, fmt.Errorf("unexpected numstat row %q", row)
			}
		}
		changes = append(changes, c)
	}
	return changes, nil
}

func (g Git) PatchAgainst(ctx context.Context, dir, base string) (string, error) {
	if _, err := g.run(ctx, dir, "", "add", "-A"); err != nil {
		return "", err
	}
	return g.run(ctx, dir, "", "diff", "--cached", "--no-color", base, "--")
}

func (g Git) Commit(ctx context.Context, dir, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errors.New("a commit needs a message")
	}
	if _, err := g.run(ctx, dir, "", "add", "-A"); err != nil {
		return "", err
	}
	if _, err := g.run(ctx, dir, message, "commit", "--quiet", "--allow-empty-message", "-F", "-"); err != nil {
		return "", err
	}
	return g.Head(ctx, dir)
}

func (g Git) ResetTo(ctx context.Context, dir, ref string) error {
	if _, err := g.run(ctx, dir, "", "reset", "--hard", "--quiet", ref); err != nil {
		return err
	}
	_, err := g.run(ctx, dir, "", "clean", "-fdq")
	return err
}

func (g Git) Push(ctx context.Context, dir, remote, branch string) error {
	if branch == "" || strings.HasPrefix(branch, "+") || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, ": \t\n") {
		return fmt.Errorf("%w: %q", ErrForcePush, branch)
	}
	_, err := g.run(ctx, dir, "", "push", "--quiet", remote, "HEAD:refs/heads/"+branch)
	return err
}

func (g Git) Show(ctx context.Context, dir, ref, path string) (string, error) {
	return g.run(ctx, dir, "", "show", ref+":"+path)
}
