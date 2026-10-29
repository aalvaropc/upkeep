package forecast

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/sandbox"
)

const libProject = `package app

import (
	"fmt"

	"example.com/lib/v2"
	"example.com/lib/v2/codec"
)

func run(tok lib.Token) {
	_, _ = lib.Parse("x")
	_ = lib.Verify(1)
	lib.Legacy()
	_ = lib.Claims{}
	_ = lib.Options{}
	_ = lib.ErrExpired
	_ = lib.LevelHigh
	_ = lib.Keys(map[string]int{})
	_ = lib.OnlyInTests
	_ = lib.Wrap(nil)
	_ = codec.Encode(nil)
	_ = codec.Decode(nil, nil)
	fmt.Println(tok.Get("k"))
}
`

var libChanges = []Change{
	{Symbol: "codec.Encode", Description: "signature changed: func Encode(v any) []byte → func Encode(v any) ([]byte, error)"},
	{Symbol: "lib.Claims", Description: "definition changed: type Claims struct{Subject string; Expires int64} → type Claims struct{Subject string; ExpiresAt int64}"},
	{Symbol: "lib.Legacy", Description: "removed in v2.0.0"},
	{Symbol: "lib.Verify", Description: "signature changed: func Verify(a int) error → func Verify(ctx context.Context, a int) error"},
}

func testdataAPI(t *testing.T, version string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "api", version))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCompareAPIReportsOnlyWhatTheProjectUsesAndChanged(t *testing.T) {
	project := writeTree(t, map[string]string{"app.go": libProject})
	refs := references(project, "example.com/lib", "example.com/lib/v2")
	changes, compared := compareAPI(testdataAPI(t, "v1.0.0"), testdataAPI(t, "v2.0.0"), "v2.0.0", refs)
	if !slices.Equal(changes, libChanges) {
		t.Fatalf("changes =\n%v\nwant\n%v", changes, libChanges)
	}
	if compared != 12 {
		t.Fatalf("compared %d identifiers, want 12", compared)
	}
}

func TestCompareAPITreatsAMissingPackageAsRemoved(t *testing.T) {
	refs := []reference{{Symbol: "codec.Encode", Sub: "codec", Ident: "Encode"}, {Symbol: "x.Y", Sub: "absent", Ident: "Y"}}
	changes, compared := compareAPI(testdataAPI(t, "v1.0.0"), t.TempDir(), "v3.0.0", refs)
	if compared != 1 || !slices.Equal(changes, []Change{{Symbol: "codec.Encode", Description: "removed in v3.0.0"}}) {
		t.Fatalf("changes %v, compared %d", changes, compared)
	}
}

func TestDescribeKeepsLongDeclarationsShort(t *testing.T) {
	long := strings.Repeat("x", maxDeclChars)
	got := describe(decl{kind: declType, key: "a", show: "type A struct{" + long + "}"}, decl{kind: declType, key: "b", show: "type A int"}, "v2.0.0")
	if got != "definition changed in v2.0.0" {
		t.Fatalf("describe = %q", got)
	}
	if got := describe(decl{kind: declFunc, key: "func A()", show: "func A()"}, decl{kind: declValue, show: "var A"}, "v2.0.0"); got != "declaration changed: func A() → var A" {
		t.Fatalf("describe = %q", got)
	}
}

func TestModulePathFollowsTheMajorVersion(t *testing.T) {
	for _, c := range []struct{ module, version, want string }{
		{"github.com/golang-jwt/jwt/v5", "v4.5.2", "github.com/golang-jwt/jwt/v4"},
		{"github.com/golang-jwt/jwt/v5", "v5.3.1", "github.com/golang-jwt/jwt/v5"},
		{"example.com/lib/v2", "v1.0.0", "example.com/lib"},
		{"example.com/lib", "v2.0.0", "example.com/lib/v2"},
		{"example.com/lib", "v0.5.0", "example.com/lib"},
		{"example.com/old", "v3.1.0+incompatible", "example.com/old"},
		{"gopkg.in/yaml.v3", "v3.0.1", "gopkg.in/yaml.v3"},
		{"example.com/lib", "garbage", "example.com/lib"},
	} {
		if got := modulePath(c.module, c.version); got != c.want {
			t.Errorf("modulePath(%q, %q) = %q, want %q", c.module, c.version, got, c.want)
		}
	}
}

func TestHostPathMapsTheDockerCacheOnly(t *testing.T) {
	d := sandbox.Docker{CacheDir: "/data/cache/go"}
	if got := hostPath(d, "/cache/gopath/pkg/mod/example.com/lib@v1.0.0"); got != "/data/cache/go/gopath/pkg/mod/example.com/lib@v1.0.0" {
		t.Fatalf("docker host path = %q", got)
	}
	if got := hostPath(sandbox.Exec{}, "/home/me/go/pkg/mod/example.com/lib@v1.0.0"); got != "/home/me/go/pkg/mod/example.com/lib@v1.0.0" {
		t.Fatalf("exec host path = %q", got)
	}
}

func TestModulesParsesTheDownloadOutput(t *testing.T) {
	out := "go: noise before\n{\n\t\"Path\": \"a\",\n\t\"Version\": \"v1.0.0\",\n\t\"Dir\": \"/m/a@v1.0.0\",\n\t\"Origin\": {\"VCS\": \"git\"}\n}\nnot json {\n{\"Path\": \"b\", \"Version\": \"v2.0.0\", \"Error\": \"b@v2.0.0: invalid version\"}\n"
	got := modules(out)
	want := []downloaded{{Path: "a", Version: "v1.0.0", Dir: "/m/a@v1.0.0"}, {Path: "b", Version: "v2.0.0", Error: "b@v2.0.0: invalid version"}}
	if !slices.Equal(got, want) {
		t.Fatalf("modules = %+v", got)
	}
}

type fakeExecutor struct {
	dirs   map[string]string
	output string
	err    error
	specs  []sandbox.Spec
	prefix string
	mapTo  string
}

func (f *fakeExecutor) Run(_ context.Context, spec sandbox.Spec) (sandbox.Result, error) {
	f.specs = append(f.specs, spec)
	if f.err != nil {
		return sandbox.Result{}, f.err
	}
	if f.output != "" {
		return sandbox.Result{Output: f.output, ExitCode: 1}, nil
	}
	var b strings.Builder
	for _, arg := range spec.Args[4:] {
		mod, version, _ := strings.Cut(arg, "@")
		fmt.Fprintf(&b, "{\n\t\"Path\": %q,\n\t\"Version\": %q,\n\t\"Dir\": %q\n}\n", mod, version, f.prefix+f.dirs[arg])
	}
	return sandbox.Result{Output: b.String()}, nil
}

func (f *fakeExecutor) HostPath(p string) string {
	if f.prefix == "" {
		return p
	}
	return f.mapTo + strings.TrimPrefix(p, f.prefix)
}

func apiForecaster(t *testing.T, exec sandbox.Executor) (*Forecaster, *fakeModel) {
	t.Helper()
	f, rel, _, m := newForecaster(t)
	rel.list = nil
	f.Search = nil
	f.Executor = exec
	return f, m
}

func libInput(t *testing.T) Input {
	return Input{Module: "example.com/lib/v2", From: "v1.0.0", To: "v2.0.0", Dir: writeTree(t, map[string]string{"app.go": libProject})}
}

func TestWithoutSourcesTheForecastComesFromTheModuleAPI(t *testing.T) {
	exec := &fakeExecutor{
		prefix: "/cache/gopath/pkg/mod",
		mapTo:  filepath.Dir(testdataAPI(t, "v1.0.0")),
		dirs:   map[string]string{"example.com/lib@v1.0.0": "/v1.0.0", "example.com/lib/v2@v2.0.0": "/v2.0.0"},
	}
	f, m := apiForecaster(t, exec)
	in := libInput(t)
	got, cost, err := f.Forecast(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if m.calls.Load() != 0 || cost.Model != "" || cost.Cached {
		t.Fatalf("the API forecast needs no model: %d calls, cost %+v", m.calls.Load(), cost)
	}
	if got.Source != SourceAPI || !slices.Equal(got.Breaking, libChanges) || !slices.Equal(got.Files, []string{"app.go"}) || got.Sources == nil || len(got.Sources) != 0 {
		t.Fatalf("forecast = %+v", got)
	}
	if !strings.Contains(got.Summary, "for the 12 identifiers the project uses from it; 4 changed") {
		t.Fatalf("summary = %q", got.Summary)
	}
	if len(exec.specs) != 1 {
		t.Fatalf("%d commands, want 1", len(exec.specs))
	}
	spec := exec.specs[0]
	if !slices.Equal(spec.Args, []string{"go", "mod", "download", "-json", "example.com/lib@v1.0.0", "example.com/lib/v2@v2.0.0"}) || !spec.Network {
		t.Fatalf("spec = %+v", spec)
	}
	if spec.Dir == in.Dir || strings.HasPrefix(spec.Dir, in.Dir) {
		t.Fatalf("the download must not run inside the pull request: %s", spec.Dir)
	}
	want := "Upgrade of example.com/lib/v2 from v1.0.0 to v2.0.0.\n\nExported API changes between v1.0.0 and v2.0.0, computed from the module source:\n- codec.Encode: signature changed: func Encode(v any) []byte → func Encode(v any) ([]byte, error)\n- lib.Claims: definition changed: type Claims struct{Subject string; Expires int64} → type Claims struct{Subject string; ExpiresAt int64}\n- lib.Legacy: removed in v2.0.0\n- lib.Verify: signature changed: func Verify(a int) error → func Verify(ctx context.Context, a int) error"
	if notes := got.Notes(); notes != want {
		t.Fatalf("Notes =\n%s\nwant\n%s", notes, want)
	}
	again, cost, err := f.Forecast(t.Context(), in)
	if err != nil || !cost.Cached || again.Source != SourceAPI || len(exec.specs) != 1 {
		t.Fatalf("the API forecast is cached like any other: %+v %+v %v", again, cost, err)
	}
}

func TestTheAPIFallbackRunsOutsideDockerWithHostPaths(t *testing.T) {
	exec := &fakeExecutor{dirs: map[string]string{"example.com/lib@v1.0.0": testdataAPI(t, "v1.0.0"), "example.com/lib/v2@v2.0.0": testdataAPI(t, "v2.0.0")}}
	f, _ := apiForecaster(t, exec)
	got, _, err := f.Forecast(t.Context(), libInput(t))
	if err != nil || !slices.Equal(got.Breaking, libChanges) {
		t.Fatalf("forecast %+v, error %v", got.Breaking, err)
	}
}

func TestTheAPIFallbackSaysWhyItFoundNothing(t *testing.T) {
	for name, c := range map[string]struct {
		exec *fakeExecutor
		in   func(t *testing.T) Input
		want string
	}{
		"download error": {
			exec: &fakeExecutor{output: "{\"Path\": \"example.com/lib/v2\", \"Version\": \"v2.0.0\", \"Error\": \"example.com/lib/v2@v2.0.0: unknown revision\"}\n"},
			in:   libInput,
			want: "the module source could not be compared: example.com/lib/v2@v2.0.0: unknown revision",
		},
		"executor error": {
			exec: &fakeExecutor{err: errors.New("docker is down")},
			in:   libInput,
			want: "the module source could not be compared: docker is down",
		},
		"nothing returned": {
			exec: &fakeExecutor{output: "go: network unreachable\n"},
			in:   libInput,
			want: "go mod download did not return example.com/lib@v1.0.0: go: network unreachable",
		},
		"no identifiers used": {
			exec: &fakeExecutor{},
			in: func(t *testing.T) Input {
				return Input{Module: "example.com/lib/v2", From: "v1.0.0", To: "v2.0.0", Dir: writeTree(t, map[string]string{"main.go": "package main\n"})}
			},
			want: "the project uses no identifier from it",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, m := apiForecaster(t, c.exec)
			_, _, err := f.Forecast(t.Context(), c.in(t))
			if err == nil || !strings.Contains(err.Error(), "no release notes or migration guide could be found") || !strings.Contains(err.Error(), c.want) || m.calls.Load() != 0 {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestTheAPIFallbackNeverRunsWhenThereAreSources(t *testing.T) {
	exec := &fakeExecutor{}
	f, _, _, m := newForecaster(t)
	f.Executor = exec
	got, _, err := f.Forecast(t.Context(), jwtInput(""))
	if err != nil || got.Source != "" || len(exec.specs) != 0 || m.calls.Load() != 1 {
		t.Fatalf("source %q, %d commands, %d model calls, error %v", got.Source, len(exec.specs), m.calls.Load(), err)
	}
}
