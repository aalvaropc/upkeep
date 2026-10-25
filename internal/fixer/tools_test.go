package fixer

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
}

func files(t *testing.T, in map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"go.mod": "module example.test/m\n\ngo 1.25\n"}
	for k, v := range in {
		all[k] = v
	}
	for name, content := range all {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newBox(t *testing.T, dir string) *toolbox {
	t.Helper()
	box, err := newToolbox(dir, sandbox.GoRunner{Executor: sandbox.Exec{}}, sandbox.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(box.close)
	return box
}

func args(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func call(box *toolbox, name, arguments string) toolResult {
	return box.call(nilContext(), model.ToolCall{ID: "c", Name: name, Arguments: arguments})
}

const serviceSource = "package m\n\nfunc Sum(a, b int) int { return a + b }\n"

func TestReadFile(t *testing.T) {
	dir := files(t, map[string]string{"service.go": serviceSource, "sub/deep.go": "package sub\n", "notes.txt": "text", ".hidden/x.go": "package x\n"})
	box := newBox(t, dir)
	if res := call(box, "read_file", args(t, map[string]string{"path": "service.go"})); !res.ok || res.text != serviceSource {
		t.Fatalf("read_file = %+v", res)
	}
	if res := call(box, "read_file", args(t, map[string]string{"path": "./sub/../sub/deep.go"})); !res.ok {
		t.Fatalf("a path that cleans to a file inside should work: %+v", res)
	}
	for name, path := range map[string]string{
		"missing":   "nope.go",
		"not go":    "notes.txt",
		"escape":    "../outside.go",
		"deep":      "sub/../../outside.go",
		"absolute":  "/etc/passwd",
		"hidden":    ".hidden/x.go",
		"git":       ".git/config",
		"backslash": `sub\deep.go`,
		"empty":     "",
		"dot":       ".",
	} {
		if res := call(box, "read_file", args(t, map[string]string{"path": path})); res.ok || !strings.HasPrefix(res.text, "error: ") {
			t.Errorf("%s: read_file(%q) = %+v, want an error", name, path, res)
		}
	}
}

func TestReadFileDoesNotFollowSymlinksOutside(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.go")
	if err := os.WriteFile(secret, []byte("package secret // private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := files(t, map[string]string{"service.go": serviceSource})
	if err := os.Symlink(secret, filepath.Join(dir, "link.go")); err != nil {
		t.Fatal(err)
	}
	box := newBox(t, dir)
	if res := call(box, "read_file", args(t, map[string]string{"path": "link.go"})); res.ok || strings.Contains(res.text, "private") {
		t.Fatalf("a symlink out of the repository was followed: %+v", res)
	}
	if res := call(box, "apply_edit", args(t, map[string]string{"path": "link.go", "old": "private", "new": "public"})); res.ok {
		t.Fatalf("a symlink out of the repository was edited: %+v", res)
	}
	if data, _ := os.ReadFile(secret); !strings.Contains(string(data), "private") {
		t.Fatal("the file outside the repository changed")
	}
}

func TestReadFileTruncatesBigFiles(t *testing.T) {
	big := "package m\n\nvar X = `" + strings.Repeat("a", maxFileChars*2) + "`\n"
	box := newBox(t, files(t, map[string]string{"big.go": big}))
	res := call(box, "read_file", args(t, map[string]string{"path": "big.go"}))
	if !res.ok || len(res.text) > maxFileChars+100 || !strings.Contains(res.text, "the file continues") {
		t.Fatalf("read_file of a big file: ok %v, %d chars", res.ok, len(res.text))
	}
}

func TestApplyEdit(t *testing.T) {
	dir := files(t, map[string]string{"service.go": serviceSource})
	box := newBox(t, dir)
	res := call(box, "apply_edit", args(t, Edit{Path: "service.go", Old: "func Sum(a, b int) int { return a + b }", New: "func Add(a, b int) int {\nreturn a+b\n}"}))
	if !res.ok || res.text != "ok" {
		t.Fatalf("apply_edit = %+v", res)
	}
	want := "package m\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	if got := read(t, dir, "service.go"); got != want {
		t.Fatalf("file =\n%q\nwant the edit applied and formatted\n%q", got, want)
	}
	if len(box.edits) != 1 || box.edits[0].Path != "service.go" {
		t.Fatalf("edits = %+v", box.edits)
	}
}

func TestApplyEditRefusals(t *testing.T) {
	dir := files(t, map[string]string{
		"service.go":         "package m\n\nfunc A() {}\n\nfunc B() {}\n\nfunc C() {}\n",
		"service_test.go":    "package m\n",
		"testdata/sample.go": "package sample\n",
		"vendor/lib/lib.go":  "package lib\n",
		"README.md":          "# m\n",
		"commented.go":       "package m\n\nfunc D() {}\n",
	})
	box := newBox(t, dir)
	cases := map[string]struct {
		edit Edit
		want string
	}{
		"test file":      {Edit{"service_test.go", "package m", "package m_test"}, "test files cannot be edited"},
		"testdata":       {Edit{"testdata/sample.go", "sample", "other"}, "testdata"},
		"vendor":         {Edit{"vendor/lib/lib.go", "lib", "other"}, "vendor"},
		"go.mod":         {Edit{"go.mod", "go 1.25", "go 1.26"}, "only Go source files"},
		"markdown":       {Edit{"README.md", "# m", "# n"}, "only Go source files"},
		"escape":         {Edit{"../x.go", "a", "b"}, "inside the repository"},
		"absolute":       {Edit{"/tmp/x.go", "a", "b"}, "relative"},
		"hidden":         {Edit{".git/hooks/x.go", "a", "b"}, "hidden"},
		"empty old":      {Edit{"service.go", "", "x"}, "must not be empty"},
		"missing text":   {Edit{"service.go", "func Z() {}", "func Y() {}"}, "found 0 times"},
		"ambiguous":      {Edit{"service.go", "() {}", "() { return }"}, "found 3 times"},
		"missing file":   {Edit{"nope.go", "a", "b"}, "cannot be read"},
		"adds a comment": {Edit{"commented.go", "func D() {}", "func D() {} // does nothing"}, "adds a comment"},
		"adds a block":   {Edit{"commented.go", "func D() {}", "/" + "* nothing *" + "/\nfunc D() {}"}, "adds a comment"},
	}
	before := map[string]string{}
	for name := range map[string]bool{"service.go": true, "commented.go": true, "service_test.go": true} {
		before[name] = read(t, dir, name)
	}
	for name, tc := range cases {
		res := call(box, "apply_edit", args(t, tc.edit))
		if res.ok || !strings.Contains(res.text, tc.want) {
			t.Errorf("%s: %+v, want an error containing %q", name, res, tc.want)
		}
	}
	for name, content := range before {
		if got := read(t, dir, name); got != content {
			t.Errorf("%s changed after refused edits", name)
		}
	}
	if len(box.edits) != 0 {
		t.Fatalf("refused edits were recorded: %+v", box.edits)
	}
}

func TestApplyEditMayRemoveComments(t *testing.T) {
	dir := files(t, map[string]string{"old.go": "package m\n\n// legacy note\nfunc E() {}\n"})
	box := newBox(t, dir)
	res := call(box, "apply_edit", args(t, Edit{Path: "old.go", Old: "// legacy note\n", New: ""}))
	if !res.ok {
		t.Fatalf("removing a comment should be allowed: %+v", res)
	}
	if got := read(t, dir, "old.go"); strings.Contains(got, "//") {
		t.Fatalf("file = %q", got)
	}
}

func TestApplyEditAllowsEmbedDirectives(t *testing.T) {
	dir := files(t, map[string]string{"web.go": "package m\n\nimport \"embed\"\n\nvar files embed.FS\n"})
	box := newBox(t, dir)
	res := call(box, "apply_edit", args(t, Edit{Path: "web.go", Old: "var files embed.FS", New: "//go:embed dist\nvar files embed.FS"}))
	if !res.ok {
		t.Fatalf("an embed directive is not a comment: %+v", res)
	}
}

func TestApplyEditReportsABrokenFile(t *testing.T) {
	dir := files(t, map[string]string{"service.go": serviceSource})
	box := newBox(t, dir)
	res := call(box, "apply_edit", args(t, Edit{Path: "service.go", Old: "return a + b", New: "return a +"}))
	if !res.ok || !strings.Contains(res.text, "no longer parses") {
		t.Fatalf("a broken edit should be applied with a warning: %+v", res)
	}
	if !strings.Contains(read(t, dir, "service.go"), "return a +") {
		t.Fatal("the edit was not written")
	}
}

func TestApplyEditPreservesFileMode(t *testing.T) {
	dir := files(t, map[string]string{"service.go": serviceSource})
	if err := os.Chmod(filepath.Join(dir, "service.go"), 0o640); err != nil {
		t.Fatal(err)
	}
	box := newBox(t, dir)
	if res := call(box, "apply_edit", args(t, Edit{Path: "service.go", Old: "a + b", New: "a - b"})); !res.ok {
		t.Fatal(res.text)
	}
	if info, _ := os.Stat(filepath.Join(dir, "service.go")); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestGoDoc(t *testing.T) {
	requireGo(t)
	box := newBox(t, files(t, map[string]string{"service.go": serviceSource}))
	res := call(box, "go_doc", args(t, map[string]string{"package": "strings", "symbol": "Builder"}))
	if !res.ok || !strings.Contains(res.text, "type Builder struct") {
		t.Fatalf("go_doc = %+v", res)
	}
	if res := call(box, "go_doc", args(t, map[string]string{"package": "example.invalid/none"})); res.ok {
		t.Fatalf("an unknown package returned documentation: %+v", res)
	}
	for _, bad := range []map[string]string{
		{"package": "-all"},
		{"package": "strings; rm -rf /"},
		{"package": "strings", "symbol": "Builder --help"},
		{"package": ""},
		{"package": "a b"},
	} {
		if res := call(box, "go_doc", args(t, bad)); res.ok || !strings.Contains(res.text, "invalid") {
			t.Errorf("go_doc(%v) = %+v, want it refused", bad, res)
		}
	}
}

func TestRunChecks(t *testing.T) {
	requireGo(t)
	dir := files(t, map[string]string{"service.go": "package m\n\nfunc Sum(a, b int) int { return missing }\n"})
	box := newBox(t, dir)
	res := call(box, "run_checks", "{}")
	if !res.ok || !strings.HasPrefix(res.text, "go build ./... failed:") || !strings.Contains(res.text, "undefined: missing") {
		t.Fatalf("run_checks = %+v", res)
	}
	if box.passed || box.checks != 1 || !strings.Contains(box.failure, "undefined") {
		t.Fatalf("state: passed %v checks %d failure %q", box.passed, box.checks, box.failure)
	}
	if res := call(box, "apply_edit", args(t, Edit{Path: "service.go", Old: "return missing", New: "return a + b"})); !res.ok {
		t.Fatal(res.text)
	}
	if res := call(box, "run_checks", ""); !res.ok || res.text != "PASS" || !box.passed {
		t.Fatalf("after the fix: %+v passed %v", res, box.passed)
	}
	if res := call(box, "apply_edit", args(t, Edit{Path: "service.go", Old: "a + b", New: "a - b"})); !res.ok {
		t.Fatal(res.text)
	}
	if box.passed {
		t.Fatal("an edit after a passing check must invalidate it")
	}
}

func TestRunChecksTruncatesLongFailures(t *testing.T) {
	requireGo(t)
	noisy := "package m\n\nimport (\n\t\"strings\"\n\t\"testing\"\n)\n\nfunc TestNoisy(t *testing.T) {\n\tt.Error(strings.Repeat(\"y\", 12000))\n}\n"
	box := newBox(t, files(t, map[string]string{"service.go": serviceSource, "noisy_test.go": noisy}))
	res := call(box, "run_checks", "{}")
	if !res.ok || !strings.HasPrefix(res.text, "go test") || !strings.Contains(res.text, "the output continues") || len(res.text) > maxReportChars+100 {
		t.Fatalf("run_checks output has %d characters: %.80s", len(res.text), res.text)
	}
}

func TestUnknownToolsAndBadArguments(t *testing.T) {
	box := newBox(t, files(t, map[string]string{"service.go": serviceSource}))
	if res := call(box, "rm_rf", "{}"); res.ok || !strings.Contains(res.text, "unknown tool") {
		t.Errorf("unknown tool = %+v", res)
	}
	for _, name := range []string{"read_file", "apply_edit", "go_doc"} {
		if res := call(box, name, "{not json"); res.ok || !strings.Contains(res.text, "not valid JSON") {
			t.Errorf("%s with bad JSON = %+v", name, res)
		}
	}
	if box.errors != 4 {
		t.Errorf("errors counted = %d, want 4", box.errors)
	}
}

func TestMentionedFilesAndPrompt(t *testing.T) {
	dir := files(t, map[string]string{
		"auth/token.go":      "package auth\n",
		"api/notes.go":       "package api\n",
		"auth/token_test.go": "package auth\n",
		"README.md":          "#",
	})
	out := "# example.test/m/auth\nauth/token.go:24:6: undefined: jwt.StandardClaims\nauth/token.go:55:23: again\napi/notes.go:27:23: x\nauth/token_test.go:9:1: test\nmissing/file.go:1:1: gone\n../outside.go:3:3: escape\n"
	got := MentionedFiles(dir, out)
	if len(got) != 2 || got[0] != "auth/token.go" || got[1] != "api/notes.go" {
		t.Fatalf("MentionedFiles = %v", got)
	}
	prompt := BuildPrompt(Task{Dir: dir, Failure: out, Notes: "v5 renames StandardClaims", Prior: "go test failed"})
	for _, want := range []string{"undefined: jwt.StandardClaims", "Contents of auth/token.go:", "Contents of api/notes.go:", "v5 renames StandardClaims", "previous attempt", "go test failed", "Fix the repository."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not contain %q", want)
		}
	}
	if strings.Contains(prompt, "Contents of auth/token_test.go") {
		t.Error("test files must not be included")
	}
	plain := BuildPrompt(Task{Dir: dir, Failure: "boom"})
	if strings.Contains(plain, "Release notes") || strings.Contains(plain, "previous attempt") {
		t.Errorf("optional sections appeared: %s", plain)
	}
}
