package gitx

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func checkRevisions(revs ...string) error {
	for _, rev := range revs {
		if rev == "" || strings.HasPrefix(rev, "-") {
			return fmt.Errorf("invalid revision %q", rev)
		}
	}
	return nil
}

func (g Git) Touched(ctx context.Context, dir, base, head string) ([]string, error) {
	if err := checkRevisions(base, head); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, dir, "", "diff", "--no-renames", "--name-only", "-z", base+"..."+head, "--")
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

func isObjectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (g Git) MergeTree(ctx context.Context, dir, a, b string) (bool, []string, error) {
	if err := checkRevisions(a, b); err != nil {
		return false, nil, err
	}
	out, err := g.run(ctx, dir, "", "merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", a, b)
	if err != nil {
		gitErr, ok := errors.AsType[*Error](err)
		if !ok || gitErr.Code != 1 {
			return false, nil, err
		}
		out = gitErr.Stdout
	}
	fields := strings.Split(out, "\x00")
	if !isObjectID(strings.TrimSpace(fields[0])) {
		if err != nil {
			return false, nil, err
		}
		return false, nil, fmt.Errorf("git merge-tree %s %s: no tree in the output", a, b)
	}
	if err == nil {
		return true, nil, nil
	}
	var files []string
	for _, f := range fields[1:] {
		if f != "" && !slices.Contains(files, f) {
			files = append(files, f)
		}
	}
	return false, files, nil
}
