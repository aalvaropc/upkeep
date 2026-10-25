package strategy

import (
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/sandbox"
)

func TestParseResolutions(t *testing.T) {
	plan, err := parseResolutions("Sure! Here you go:\n```json\n{\"files\":[{\"path\":\"./notes/limits.go\",\"resolutions\":[\"a\",\"b\"]},{\"path\":\"api/x.go\",\"resolutions\":[\"c\"]}]}\n```\nHope that helps.")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 || strings.Join(plan["notes/limits.go"], ",") != "a,b" || strings.Join(plan["api/x.go"], ",") != "c" {
		t.Fatalf("plan = %v", plan)
	}
}

func TestParseResolutionsErrors(t *testing.T) {
	for name, answer := range map[string]string{
		"no object":      "keep both",
		"reversed":       "} {",
		"invalid":        `{"files": [}`,
		"duplicate path": `{"files":[{"path":"a.go","resolutions":["x"]},{"path":"./a.go","resolutions":["y"]}]}`,
	} {
		if _, err := parseResolutions(answer); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	plan, err := parseResolutions(`{"files":[]}`)
	if err != nil || len(plan) != 0 {
		t.Fatalf("an empty plan parses: %v %v", plan, err)
	}
}

func TestFailureText(t *testing.T) {
	got := FailureText(sandbox.Result{Args: []string{"go", "build", "./..."}, Output: "\n./a.go:3:1: undefined: x\n"})
	if got != "go build ./... failed:\n./a.go:3:1: undefined: x" {
		t.Fatalf("FailureText = %q", got)
	}
}

func TestFirstLine(t *testing.T) {
	if firstLine("  one\ntwo ") != "one" || firstLine("") != "" {
		t.Fatal("firstLine is wrong")
	}
}
