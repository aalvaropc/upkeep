package policy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/pull"
)

func TestDefault(t *testing.T) {
	p := Default()
	if err := p.Validate(); err != nil {
		t.Fatalf("Default() is not valid: %v", err)
	}
	if p.MaxFilesChanged != 5 || p.MaxLinesChanged != 80 || p.MaxConflictHunks != 3 || p.MaxHunkLines != 20 ||
		p.MaxAttempts != 3 || p.MaxSeconds != 180 || p.MaxTokens != 300000 {
		t.Errorf("limits = %+v", p)
	}
	if p.AutoMerge != AutoMergeBots || !reflect.DeepEqual(p.AutoMergeBumps, []pull.Level{pull.Patch, pull.Minor}) || p.MergeMethod != "merge" {
		t.Errorf("merge settings = %+v", p)
	}
	if p.Label != "upkeep" || !reflect.DeepEqual(p.Bots, []string{"dependabot[bot]", "renovate[bot]"}) {
		t.Errorf("label and bots = %q %q", p.Label, p.Bots)
	}
}

func TestParsePartialKeepsDefaults(t *testing.T) {
	p, err := Parse(strings.NewReader("max_attempts = 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.MaxAttempts = 5
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("Parse = %+v, want %+v", p, want)
	}
}

func TestParseFull(t *testing.T) {
	src := strings.Join([]string{
		"# team policy",
		"",
		`label = "auto-fix" # opt-in label`,
		`bots = ["dependabot[bot]", "my-bot", ]`,
		`auto_merge = "none"`,
		`auto_merge_bumps = ["patch"]`,
		`merge_method = "merge"`,
		"max_files_changed = 3",
		"max_lines_changed = 40",
		"max_conflict_hunks = 2",
		"max_hunk_lines = 10",
		"max_attempts = 2",
		"max_seconds = 120",
		"max_tokens = 40_000",
	}, "\n")
	p, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := Policy{
		Label:            "auto-fix",
		Bots:             []string{"dependabot[bot]", "my-bot"},
		AutoMerge:        AutoMergeNone,
		AutoMergeBumps:   []pull.Level{pull.Patch},
		MergeMethod:      "merge",
		MaxFilesChanged:  3,
		MaxLinesChanged:  40,
		MaxConflictHunks: 2,
		MaxHunkLines:     10,
		MaxAttempts:      2,
		MaxSeconds:       120,
		MaxTokens:        40000,
	}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("Parse =\n%+v\nwant\n%+v", p, want)
	}
}

func TestStringEscapes(t *testing.T) {
	p, err := Parse(strings.NewReader(`label = "a\"b\\c\td"`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Label != "a\"b\\c\td" {
		t.Fatalf("label = %q", p.Label)
	}
}

func TestExampleFileIsTheDefault(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "examples", "upkeep.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, Default()) {
		t.Fatalf("examples/upkeep.toml =\n%+v\nDefault() =\n%+v", p, Default())
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "upkeep.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, Default()) {
		t.Fatalf("Load(missing) = %+v", p)
	}
}

func TestLoadNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upkeep.toml")
	if err := os.WriteFile(path, []byte("nope = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"colour = \"red\"", `line 1: unknown key "colour"`},
		{"max_attempts = 1\nmax_attempts = 2", "line 2: key \"max_attempts\" is already set on line 1"},
		{"max_attempts = 1_", "invalid integer"},
		{"max_attempts = 1__0", "invalid integer"},
		{"max_attempts = ten", "unsupported value"},
		{"max_attempts = \"3\"", "max_attempts must be an integer"},
		{"label = 3", "label must be a string"},
		{"bots = \"dependabot\"", "bots must be an array of strings"},
		{"label = \"open", "unterminated string"},
		{"label = 'single'", "literal strings are not supported"},
		{"label = \"x\\q\"", "unsupported escape"},
		{"bots = [\"a\",", "unterminated array"},
		{"bots = [\"a\"\n, \"b\"]", "expected , or ]"},
		{"bots = [1]", "arrays may only contain strings"},
		{"[policy]", "tables are not supported"},
		{"label", "expected key = value"},
		{"bad key = 1", "invalid key"},
		{"label = \"x\" trailing", "unexpected"},
		{"max_attempts = 0", "max_attempts must be greater than zero"},
		{"max_tokens = 99999999999", "out of range"},
		{"auto_merge = \"always\"", "auto_merge"},
		{"auto_merge_bumps = [\"major\"]", "majors are always left to a person"},
		{"auto_merge_bumps = [\"huge\"]", "unknown level"},
		{"merge_method = \"squash\"", "only merges with a merge commit"},
		{"label = \"  \"", "label must not be empty"},
		{"force_push = true", "line 1: force_push is a hard rule"},
		{"merge_human_prs = true", "hard rule"},
		{"edit_tests = true", "hard rule"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.src))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse(%q) error = %v, want it to contain %q", tt.src, err, tt.want)
			}
		})
	}
}

func TestIsBot(t *testing.T) {
	p := Default()
	tests := []struct {
		login, accountType string
		want               bool
	}{
		{"dependabot[bot]", "Bot", true},
		{"Renovate[bot]", "User", true},
		{"github-actions[bot]", "Bot", true},
		{"alice", "User", false},
		{"dependabot", "User", false},
	}
	for _, tt := range tests {
		if got := p.IsBot(tt.login, tt.accountType); got != tt.want {
			t.Errorf("IsBot(%q, %q) = %v, want %v", tt.login, tt.accountType, got, tt.want)
		}
	}
}

func TestMayAutoMerge(t *testing.T) {
	bot := pull.Author{Login: "dependabot[bot]", Kind: pull.Bot}
	person := pull.Author{Login: "alice", Kind: pull.Person}
	bump := func(l pull.Level) *pull.Bump { return &pull.Bump{Module: "example.com/m", Level: l} }
	none := Default()
	none.AutoMerge = AutoMergeNone
	patchOnly := Default()
	patchOnly.AutoMergeBumps = []pull.Level{pull.Patch}
	tests := []struct {
		name   string
		policy Policy
		author pull.Author
		bump   *pull.Bump
		want   bool
	}{
		{"bot patch", Default(), bot, bump(pull.Patch), true},
		{"bot minor", Default(), bot, bump(pull.Minor), true},
		{"bot major", Default(), bot, bump(pull.Major), false},
		{"bot without bump", Default(), bot, nil, false},
		{"person patch", Default(), person, bump(pull.Patch), false},
		{"auto merge off", none, bot, bump(pull.Patch), false},
		{"minor not allowed", patchOnly, bot, bump(pull.Minor), false},
	}
	for _, tt := range tests {
		if got := tt.policy.MayAutoMerge(tt.author, tt.bump); got != tt.want {
			t.Errorf("%s: MayAutoMerge = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestMayPush(t *testing.T) {
	p := Default()
	if !p.MayPush(pull.Request{Author: pull.Author{Kind: pull.Bot}}) {
		t.Error("a bot's branch should accept fixes")
	}
	if p.MayPush(pull.Request{Author: pull.Author{Kind: pull.Person}}) {
		t.Error("a person's branch without the label should not accept fixes")
	}
	if !p.MayPush(pull.Request{Author: pull.Author{Kind: pull.Person}, Labels: []string{"Upkeep"}}) {
		t.Error("a person's branch with the label should accept fixes")
	}
	if p.MayPush(pull.Request{Author: pull.Author{Kind: pull.Person}, Labels: []string{"upkeep"}, Fork: true}) {
		t.Error("a branch in a fork never accepts pushes, even with the label")
	}
}

func TestRules(t *testing.T) {
	rules := strings.Join(Default().Rules(), "\n")
	for _, want := range []string{"Never force-pushes", "opened by a person", `"upkeep" label`, "patch or minor", "at most 5 files and 80 lines", "3 attempts"} {
		if !strings.Contains(rules, want) {
			t.Errorf("Rules() does not mention %q:\n%s", want, rules)
		}
	}
	none := Default()
	none.AutoMerge = AutoMergeNone
	if !strings.Contains(strings.Join(none.Rules(), "\n"), "Never merges anything on its own") {
		t.Error("Rules() with auto_merge none does not say so")
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"max_attempts = 3",
		`bots = ["a", "b",]`,
		`label = "x\"y"`,
		"[t]",
		"max_tokens = 1_000",
		"auto_merge_bumps = [\"patch\"] # c",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		p, err := Parse(strings.NewReader(src))
		if err == nil {
			if verr := p.Validate(); verr != nil {
				t.Fatalf("Parse accepted an invalid policy: %v", verr)
			}
		}
	})
}
