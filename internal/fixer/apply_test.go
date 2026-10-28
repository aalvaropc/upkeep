package fixer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const callerSource = "package m\n\nfunc Total() int { return Add(1, 2) }\n"

func TestApplyEditsReplaysTheEditsInOrder(t *testing.T) {
	dir := files(t, map[string]string{"service.go": serviceSource, "caller.go": callerSource})
	edits := []Edit{
		{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1,   2)"},
		{Path: "caller.go", Old: "func Total()", New: "func Total2()"},
		{Path: "service.go", Old: "a + b", New: "b + a"},
	}
	if err := ApplyEdits(dir, edits); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "caller.go"); got != "package m\n\nfunc Total2() int { return Sum(1, 2) }\n" {
		t.Fatalf("caller.go = %q", got)
	}
	if got := read(t, dir, "service.go"); got != "package m\n\nfunc Sum(a, b int) int { return b + a }\n" {
		t.Fatalf("service.go = %q", got)
	}
}

func TestApplyEditsMatchesWhatTheToolboxWrote(t *testing.T) {
	dir := files(t, map[string]string{"caller.go": callerSource})
	box := newBox(t, dir)
	edits := []Edit{{Path: "caller.go", Old: "Add(1, 2)", New: "Sum( 1,2 )"}, {Path: "caller.go", Old: "Sum(1, 2)", New: "Sum(2, 1)"}}
	for _, e := range edits {
		if res := call(box, "apply_edit", args(t, e)); !res.ok {
			t.Fatalf("apply_edit = %+v", res)
		}
	}
	want := read(t, dir, "caller.go")
	again := files(t, map[string]string{"caller.go": callerSource})
	if err := ApplyEdits(again, box.edits); err != nil {
		t.Fatal(err)
	}
	if got := read(t, again, "caller.go"); got != want {
		t.Fatalf("replayed %q, the toolbox wrote %q", got, want)
	}
}

func TestApplyEditsIsAllOrNothing(t *testing.T) {
	cases := map[string][]Edit{
		"no edits":         nil,
		"a missing match":  {{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}, {Path: "service.go", Old: "a * b", New: "a + b"}},
		"two matches":      {{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}, {Path: "service.go", Old: "a", New: "x"}},
		"a test file":      {{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}, {Path: "service_test.go", Old: "package m", New: "package n"}},
		"a vendored file":  {{Path: "vendor/x/x.go", Old: "package x", New: "package y"}},
		"a path outside":   {{Path: "../caller.go", Old: "Add", New: "Sum"}},
		"an empty old":     {{Path: "caller.go", Old: "", New: "x"}},
		"a missing file":   {{Path: "gone.go", Old: "x", New: "y"}},
		"an added comment": {{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}, {Path: "service.go", Old: "a + b", New: "a + b " + "/" + "* sum *" + "/"}},
		"a broken file":    {{Path: "caller.go", Old: "Add(1, 2)", New: "Sum(1, 2)"}, {Path: "service.go", Old: "{ return a + b }", New: "{ return a + "}},
	}
	for name, edits := range cases {
		t.Run(name, func(t *testing.T) {
			dir := files(t, map[string]string{
				"service.go":      serviceSource,
				"caller.go":       callerSource,
				"service_test.go": "package m\n",
				"vendor/x/x.go":   "package x\n",
			})
			if err := ApplyEdits(dir, edits); err == nil {
				t.Fatal("the edits were accepted")
			}
			if read(t, dir, "caller.go") != callerSource || read(t, dir, "service.go") != serviceSource {
				t.Fatal("a refused set of edits changed a file")
			}
		})
	}
}

func TestApplyEditsKeepsTheFileMode(t *testing.T) {
	dir := files(t, map[string]string{"caller.go": callerSource})
	path := filepath.Join(dir, "caller.go")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := ApplyEdits(dir, []Edit{{Path: "caller.go", Old: "Add", New: "Sum"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 || !strings.Contains(read(t, dir, "caller.go"), "Sum(1, 2)") {
		t.Fatalf("mode %v content %q", info.Mode().Perm(), read(t, dir, "caller.go"))
	}
}
