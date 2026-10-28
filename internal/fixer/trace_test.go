package fixer

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/model"
)

type traced struct {
	kind events.Kind
	data map[string]any
}

type tracer struct {
	mu     sync.Mutex
	events []traced
}

func (r *tracer) emit(kind events.Kind, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, _ := data.(map[string]any)
	r.events = append(r.events, traced{kind: kind, data: m})
}

func (r *tracer) of(kind events.Kind) []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]any
	for _, e := range r.events {
		if e.kind == kind {
			out = append(out, e.data)
		}
	}
	return out
}

func tracedScript() []step {
	return []step{
		{
			content: "The caller still uses Add.\nLet me look.",
			calls: []model.ToolCall{
				tc("1", "read_file", map[string]string{"path": "caller.go"}),
				tc("2", "go_doc", map[string]string{"package": "strings", "symbol": "Cut"}),
				tc("3", "run_checks", "{}"),
			},
		},
		{content: "I think it is fine now."},
		{calls: []model.ToolCall{
			tc("4", "apply_edit", Edit{Path: "caller_test.go", Old: "x", New: "y"}),
			tc("5", "apply_edit", Edit{Path: "caller.go", Old: "package m\n\nfunc Total() int { return Add(1, 2) }", New: "package m\n\nfunc Total() int {\n\treturn Sum(1, 2)\n}"}),
			tc("6", "run_checks", "{}"),
		}},
	}
}

func requestKeys(t *testing.T, reqs []model.Request) []string {
	t.Helper()
	keys := make([]string, len(reqs))
	for i, req := range reqs {
		key, err := model.RequestKey(req)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}
	return keys
}

func TestTracingNeverChangesTheRequests(t *testing.T) {
	requireGo(t)
	plainDir := renameModule(t)
	plain := &scriptedModel{steps: tracedScript()}
	quiet := newFixer(plain, nil)
	first := quiet.Run(t.Context(), Task{Dir: plainDir, Failure: failureOf(t, plainDir), Notes: "a note", Prior: "an earlier failure"})

	tracedDir := renameModule(t)
	watched := &scriptedModel{steps: tracedScript()}
	rec := &tracer{}
	loud := newFixer(watched, nil)
	loud.Emit = rec.emit
	loud.Attempt = 2
	second := loud.Run(t.Context(), Task{Dir: tracedDir, Failure: failureOf(t, tracedDir), Notes: "a note", Prior: "an earlier failure"})

	if !first.Success || !second.Success {
		t.Fatalf("attempts = %+v and %+v", first, second)
	}
	plainKeys, tracedKeys := requestKeys(t, plain.requests), requestKeys(t, watched.requests)
	if len(plainKeys) != 3 || !slices.Equal(plainKeys, tracedKeys) {
		t.Fatalf("the request keys differ with tracing on:\n%v\n%v", plainKeys, tracedKeys)
	}
	if len(rec.of(events.ModelCall)) != 3 || len(rec.of(events.ToolCall)) != 6 {
		t.Fatalf("events = %+v", rec.events)
	}
}

func TestModelCallsCarryTheTurn(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	rec := &tracer{}
	f := newFixer(&scriptedModel{steps: tracedScript()}, nil)
	f.Emit = rec.emit
	f.Attempt = 2
	if a := f.Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)}); !a.Success {
		t.Fatalf("attempt = %+v", a)
	}
	calls := rec.of(events.ModelCall)
	first := calls[0]
	if first["step"] != "repair" || first["purpose"] != "fix" || first["attempt"] != 2 || first["turn"] != 1 {
		t.Fatalf("first call = %+v", first)
	}
	if first["said"] != "The caller still uses Add. Let me look." {
		t.Fatalf("said = %q", first["said"])
	}
	if tools, _ := first["tools"].([]string); !slices.Equal(tools, []string{"read_file", "go_doc", "run_checks"}) {
		t.Fatalf("tools = %v", first["tools"])
	}
	if tools, _ := calls[1]["tools"].([]string); tools == nil || len(tools) != 0 || calls[1]["turn"] != 2 {
		t.Fatalf("a reply without tools = %+v", calls[1])
	}
	if calls[2]["turn"] != 3 {
		t.Fatalf("third call = %+v", calls[2])
	}
}

func TestToolCallsNameWhatTheyTouched(t *testing.T) {
	requireGo(t)
	dir := renameModule(t)
	rec := &tracer{}
	f := newFixer(&scriptedModel{steps: tracedScript()}, nil)
	f.Emit = rec.emit
	f.Attempt = 1
	if a := f.Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir)}); !a.Success {
		t.Fatalf("attempt = %+v", a)
	}
	tools := rec.of(events.ToolCall)
	if len(tools) != 6 {
		t.Fatalf("tool calls = %+v", tools)
	}
	read, doc, check, refused, edit, pass := tools[0], tools[1], tools[2], tools[3], tools[4], tools[5]
	if read["path"] != "caller.go" || read["result"] != "3 lines" || read["detail"] != "read_file" || read["attempt"] != 1 || read["turn"] != 1 {
		t.Fatalf("read_file = %+v", read)
	}
	if doc["package"] != "strings" || doc["symbol"] != "Cut" || doc["ok"] != true || !strings.Contains(doc["result"].(string), "Cut") {
		t.Fatalf("go_doc = %+v", doc)
	}
	result, _ := check["result"].(string)
	if !strings.HasPrefix(result, "caller.go:3:") || !strings.Contains(result, "undefined: Add") {
		t.Fatalf("run_checks result = %q", result)
	}
	if detail, _ := check["detail"].(string); !strings.HasSuffix(detail, "failed:") {
		t.Fatalf("run_checks detail = %q; it must stay the header line", detail)
	}
	if refused["ok"] != false || refused["path"] != "caller_test.go" || refused["result"] != "test files cannot be edited" {
		t.Fatalf("refused edit = %+v", refused)
	}
	if _, counted := refused["added"]; counted {
		t.Fatalf("a refused edit must not count lines: %+v", refused)
	}
	if edit["path"] != "caller.go" || edit["added"] != 3 || edit["removed"] != 1 || edit["turn"] != 3 {
		t.Fatalf("apply_edit = %+v", edit)
	}
	if pass["result"] != "PASS" || pass["detail"] != "PASS" {
		t.Fatalf("passing run_checks = %+v", pass)
	}
}

func TestChangedLinesIgnoresSharedContext(t *testing.T) {
	for _, c := range []struct {
		old, updated   string
		added, removed int
	}{
		{"Add(1, 2)", "Sum(1, 2)", 1, 1},
		{"a\nb\nc", "a\nB\nc", 1, 1},
		{"a\nc", "a\nb1\nb2\nc", 2, 0},
		{"a\nb\nc", "a\nc", 0, 1},
		{"same", "same", 0, 0},
	} {
		added, removed := changedLines(c.old, c.updated)
		if added != c.added || removed != c.removed {
			t.Errorf("changedLines(%q, %q) = +%d -%d, want +%d -%d", c.old, c.updated, added, removed, c.added, c.removed)
		}
	}
}

func TestFirstErrorPrefersTheSourceLine(t *testing.T) {
	for in, want := range map[string]string{
		"go build ./... failed:\n# example.test/m\n./auth/token.go:24:15: undefined: jwt.StandardClaims\n./auth/token.go:30:2: too many errors": "auth/token.go:24:15: undefined: jwt.StandardClaims",
		"go test ./... failed:\n--- FAIL: TestX\n    x_test.go:12: got 1":                                                                       "x_test.go:12: got 1",
		"the checks did not complete": "the checks did not complete",
		"":                            "",
	} {
		if got := FirstError(in); got != want {
			t.Errorf("FirstError(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCutKeepsTraceTextShort(t *testing.T) {
	long := strings.Repeat("é", 300)
	got := Cut(long, 200)
	if n := len([]rune(got)); n != 200 || !strings.HasSuffix(got, "…") {
		t.Fatalf("cut to %d runes: %q", n, got)
	}
	if Cut("  two\n lines ", 200) != "two lines" {
		t.Fatal("whitespace must collapse")
	}
}
