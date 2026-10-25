package gitx

import (
	"context"
	"strings"

	"github.com/aalvaropc/upkeep/internal/stub"
)

type Git struct {
	Bin string
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

type FileChange struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

func (c FileChange) IsTest() bool {
	return strings.HasSuffix(c.Path, "_test.go") || strings.HasPrefix(c.Path, "testdata/") || strings.Contains(c.Path, "/testdata/")
}

func (g Git) Clone(_ context.Context, _, _ string) error {
	return stub.Err("git clone")
}

func (g Git) Merge(_ context.Context, _, _ string) (MergeResult, error) {
	return MergeResult{}, stub.Err("git merge")
}

func (g Git) IsAncestor(_ context.Context, _, _, _ string) (bool, error) {
	return false, stub.Err("git merge-base")
}

func (g Git) Diff(_ context.Context, _, _ string) ([]FileChange, error) {
	return nil, stub.Err("git diff")
}

func (g Git) Commit(_ context.Context, _, _ string) (string, error) {
	return "", stub.Err("git commit")
}

func (g Git) Push(_ context.Context, _, _, _ string) error {
	return stub.Err("git push")
}
