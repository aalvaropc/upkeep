package fixer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

const (
	maxFileChars   = 24000
	maxDocChars    = 6000
	maxReportChars = 4000
)

var (
	docPackage = regexp.MustCompile(`^[A-Za-z0-9_./~-]+$`)
	docSymbol  = regexp.MustCompile(`^[A-Za-z0-9_.]*$`)
)

func Tools() []model.Tool {
	return []model.Tool{
		{
			Name:        "read_file",
			Description: "Read a Go source file of the repository.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
		{
			Name:        "apply_edit",
			Description: "Replace one exact occurrence of old with new in a Go file. old must match exactly once. Test files cannot be edited and comments cannot be added.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"}},"required":["path","old","new"]}`),
		},
		{
			Name:        "go_doc",
			Description: "Show the real API of a package or symbol from the installed dependency versions, like go doc. Use it instead of guessing an API.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"package":{"type":"string"},"symbol":{"type":"string"}},"required":["package"]}`),
		},
		{
			Name:        "run_checks",
			Description: "Run go build, go vet and go test and return PASS or the output of the first failure.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
}

type Edit struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

type toolbox struct {
	dir      string
	root     *os.Root
	runner   sandbox.Runner
	executor sandbox.Executor
	edits    []Edit
	checks   int
	passed   bool
	last     sandbox.Report
	failure  string
	errors   int
}

func newToolbox(dir string, runner sandbox.Runner, executor sandbox.Executor) (*toolbox, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &toolbox{dir: dir, root: root, runner: runner, executor: executor}, nil
}

func (t *toolbox) close() { _ = t.root.Close() }

type toolResult struct {
	text string
	ok   bool
}

func fail(format string, args ...any) toolResult {
	return toolResult{text: "error: " + fmt.Sprintf(format, args...)}
}

func (t *toolbox) call(ctx context.Context, tc model.ToolCall) toolResult {
	var res toolResult
	switch tc.Name {
	case "read_file":
		res = t.readFile(tc.Arguments)
	case "apply_edit":
		res = t.applyEdit(tc.Arguments)
	case "go_doc":
		res = t.goDoc(ctx, tc.Arguments)
	case "run_checks":
		res = t.runChecks(ctx)
	default:
		res = fail("unknown tool %q", tc.Name)
	}
	if !res.ok {
		t.errors++
	}
	return res
}

func decode(raw string, into any) error {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		return errors.New("the arguments are not valid JSON")
	}
	return nil
}

func editablePath(name string) (string, error) {
	if name == "" || path.IsAbs(name) || strings.ContainsRune(name, '\\') {
		return "", errors.New("the path must be relative to the repository")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("the path must stay inside the repository")
	}
	for _, part := range strings.Split(clean, "/") {
		if strings.HasPrefix(part, ".") {
			return "", errors.New("hidden files and directories are not available")
		}
	}
	return clean, nil
}

func readablePath(name string) (string, error) {
	clean, err := editablePath(name)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(clean, ".go") {
		return "", errors.New("only Go source files are available")
	}
	return clean, nil
}

func (t *toolbox) readFile(raw string) toolResult {
	var args struct {
		Path string `json:"path"`
	}
	if err := decode(raw, &args); err != nil {
		return fail("%v", err)
	}
	clean, err := readablePath(args.Path)
	if err != nil {
		return fail("%v", err)
	}
	data, err := t.root.ReadFile(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fail("%s does not exist", clean)
		}
		return fail("%s cannot be read", clean)
	}
	text := string(data)
	if len(text) > maxFileChars {
		text = text[:maxFileChars] + "\n... the file continues ...\n"
	}
	return toolResult{text: text, ok: true}
}

func protected(clean string) string {
	switch {
	case strings.HasSuffix(clean, "_test.go"):
		return "test files cannot be edited"
	case !strings.HasSuffix(clean, ".go"):
		return "only Go source files can be edited"
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "testdata" || part == "vendor" {
			return "files under " + part + " cannot be edited"
		}
	}
	return ""
}

func countComments(src []byte) int {
	file := token.NewFileSet().AddFile("", -1, len(src))
	var sc scanner.Scanner
	sc.Init(file, src, func(token.Position, string) {}, scanner.ScanComments)
	n := 0
	for {
		_, tok, lit := sc.Scan()
		if tok == token.EOF {
			return n
		}
		if tok == token.COMMENT && !strings.HasPrefix(lit, "//go:embed ") {
			n++
		}
	}
}

func (t *toolbox) applyEdit(raw string) toolResult {
	var args struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		New  string `json:"new"`
	}
	if err := decode(raw, &args); err != nil {
		return fail("%v", err)
	}
	clean, err := editablePath(args.Path)
	if err != nil {
		return fail("%v", err)
	}
	if why := protected(clean); why != "" {
		return fail("%s", why)
	}
	if args.Old == "" {
		return fail("old must not be empty")
	}
	data, err := t.root.ReadFile(clean)
	if err != nil {
		return fail("%s cannot be read", clean)
	}
	text := string(data)
	if n := strings.Count(text, args.Old); n != 1 {
		return fail("old was found %d times in %s, it must match exactly once", n, clean)
	}
	updated := strings.Replace(text, args.Old, args.New, 1)
	if countComments([]byte(updated)) > countComments(data) {
		return fail("this edit adds a comment; the code must have no comments")
	}
	note := ""
	if formatted, ferr := format.Source([]byte(updated)); ferr == nil {
		updated = string(formatted)
	} else {
		note = " The file no longer parses: " + firstLine(ferr.Error())
	}
	info, err := t.root.Stat(clean)
	if err != nil {
		return fail("%s cannot be written", clean)
	}
	if err := t.root.WriteFile(clean, []byte(updated), info.Mode().Perm()); err != nil {
		return fail("%s cannot be written", clean)
	}
	t.edits = append(t.edits, Edit{Path: clean, Old: args.Old, New: args.New})
	t.passed = false
	return toolResult{text: "ok" + note, ok: true}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func (t *toolbox) goDoc(ctx context.Context, raw string) toolResult {
	var args struct {
		Package string `json:"package"`
		Symbol  string `json:"symbol"`
	}
	if err := decode(raw, &args); err != nil {
		return fail("%v", err)
	}
	if !docPackage.MatchString(args.Package) || strings.HasPrefix(args.Package, "-") || !docSymbol.MatchString(args.Symbol) {
		return fail("invalid package or symbol")
	}
	cmd := []string{"go", "doc", args.Package}
	if args.Symbol != "" {
		cmd = append(cmd, args.Symbol)
	}
	res, err := t.executor.Run(ctx, sandbox.Spec{Dir: t.dir, Args: cmd, Timeout: sandbox.DefaultTimeout})
	if err != nil {
		return fail("go doc could not run")
	}
	out := strings.TrimSpace(res.Output)
	if !res.OK() || out == "" {
		return fail("no documentation found for %s %s", args.Package, args.Symbol)
	}
	if len(out) > maxDocChars {
		out = out[:maxDocChars] + "\n... the documentation continues ..."
	}
	return toolResult{text: out, ok: true}
}

func (t *toolbox) runChecks(ctx context.Context) toolResult {
	t.checks++
	report, err := t.runner.Check(ctx, t.dir)
	if err != nil {
		return fail("the checks could not run: %v", err)
	}
	t.last = report
	if failure, found := report.FirstFailure(); found {
		text := fmt.Sprintf("%s failed:\n%s", strings.Join(failure.Args, " "), strings.TrimSpace(failure.Output))
		if len(text) > maxReportChars {
			text = text[:maxReportChars] + "\n... the output continues ..."
		}
		t.failure = text
		t.passed = false
		return toolResult{text: text, ok: true}
	}
	if !report.Passed() {
		t.failure = "the checks did not complete"
		return toolResult{text: t.failure, ok: true}
	}
	t.failure = ""
	t.passed = true
	return toolResult{text: "PASS", ok: true}
}
