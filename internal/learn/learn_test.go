package learn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/pull"
)

const jwtFailure = `go build ./... failed:
# github.com/aalvaropc/upkeep-sample/auth
auth/token.go:24:6: undefined: jwt.StandardClaims
auth/token.go:29:57: c.ExpiresAt undefined (type Claims has no field or method ExpiresAt)
auth/token.go:57:23: undefined: jwt.StandardClaims`

func TestSignature(t *testing.T) {
	cases := []struct {
		name    string
		failure string
		want    string
	}{
		{
			name:    "build errors keep the file and the message, sorted and without positions or repeats",
			failure: jwtFailure,
			want:    "auth/token.go: c.ExpiresAt undefined (type Claims has no field or method ExpiresAt)\nauth/token.go: undefined: jwt.StandardClaims",
		},
		{
			name:    "a relative prefix is cleaned",
			failure: "go build ./... failed:\n./caller.go:3:27: undefined: Add",
			want:    "caller.go: undefined: Add",
		},
		{
			name:    "a position inside the message is dropped too",
			failure: "x.go:10:2: Add redeclared in this block\n\tother declaration of Add at ./y.go:4:6",
			want:    "x.go: Add redeclared in this block",
		},
		{
			name:    "test output is indented and timed",
			failure: "go test -count=1 ./... failed:\n--- FAIL: TestAdd (0.00s)\n    service_test.go:6: wrong\nFAIL\nFAIL\texample.test/m\t0.003s",
			want:    "service_test.go: wrong",
		},
		{
			name:    "output without positions keeps its normalized lines",
			failure: "go mod download failed:\ngo: example.com/lib@v2.0.0: reading /tmp/gomod-123/cache: 404 Not Found\ntook 1.5s",
			want:    "go mod download failed:\ngo: example.com/lib@v2.0.0: reading <tmp>: 404 Not Found\ntook <duration>",
		},
		{
			name:    "an empty failure has an empty signature",
			failure: "  \n\n",
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Signature(c.failure); got != c.want {
				t.Fatalf("Signature() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestKeyIgnoresWhereTheFailureMoved(t *testing.T) {
	bump := &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major}
	moved := "go build ./... failed:\n# github.com/aalvaropc/upkeep-sample/auth\nauth/token.go:31:6: undefined: jwt.StandardClaims\nauth/token.go:36:57: c.ExpiresAt undefined (type Claims has no field or method ExpiresAt)"
	base := Key("acme/notes", bump, jwtFailure)
	if len(base) != 64 || Key("acme/notes", bump, moved) != base {
		t.Fatal("the same failure on other lines must have the same key")
	}
	later := *bump
	later.From, later.To = "v4.4.0", "v5.4.0"
	if Key("acme/notes", &later, jwtFailure) != base {
		t.Fatal("the key follows the major versions, not the minor ones")
	}
	other := *bump
	other.To = "v6.0.0"
	for name, key := range map[string]string{
		"another repository": Key("acme/other", bump, jwtFailure),
		"another major":      Key("acme/notes", &other, jwtFailure),
		"no bump":            Key("acme/notes", nil, jwtFailure),
		"another failure":    Key("acme/notes", bump, "go build ./... failed:\nauth/token.go:3:2: undefined: jwt.Parse"),
	} {
		if key == base {
			t.Errorf("%s has the same key", name)
		}
	}
}

func sample(key string, pr int) Fix {
	return Fix{
		Key:       key,
		Repo:      "acme/notes",
		Signature: "auth/token.go: undefined: jwt.StandardClaims",
		Edits:     []fixer.Edit{{Path: "auth/token.go", Old: "jwt.StandardClaims", New: "jwt.RegisteredClaims"}},
		Patch:     "diff --git a/auth/token.go b/auth/token.go\n",
		Note:      "Moves the claims to jwt.RegisteredClaims.",
		PR:        pr,
		Calls:     5,
		Micros:    6400,
		At:        time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
	}
}

func TestABookCoversTheJumpsItLearnedAFixFor(t *testing.T) {
	bump := &pull.Bump{Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1", Level: pull.Major}
	scope := Scope("acme/notes", bump)
	key := Key("acme/notes", bump, jwtFailure)
	if len(scope) != 32 || !strings.HasPrefix(key, scope) || strings.HasPrefix(Key("acme/notes", nil, jwtFailure), scope) {
		t.Fatalf("scope %q key %q", scope, key)
	}
	for name, book := range map[string]Book{"memory": &Memory{}, "dir": &Dir{Path: filepath.Join(t.TempDir(), "learned")}} {
		t.Run(name, func(t *testing.T) {
			if book.Covers(scope) || book.Covers("") {
				t.Fatal("an empty book covers a jump")
			}
			if err := book.Save(sample(key, 5)); err != nil {
				t.Fatal(err)
			}
			if !book.Covers(scope) || book.Covers(Scope("acme/other", bump)) || book.Covers("") {
				t.Fatal("a book covers the wrong jumps")
			}
		})
	}
}

func TestMemoryReturnsCopies(t *testing.T) {
	m := &Memory{}
	if _, ok := m.Find("k"); ok {
		t.Fatal("an empty memory found a fix")
	}
	if err := m.Save(Fix{}); err == nil {
		t.Fatal("a fix without a key was saved")
	}
	fix := sample("k", 5)
	if err := m.Save(fix); err != nil {
		t.Fatal(err)
	}
	fix.Edits[0].New = "changed after saving"
	got, ok := m.Find("k")
	if !ok || got.Edits[0].New != "jwt.RegisteredClaims" || got.PR != 5 || got.Micros != 6400 {
		t.Fatalf("Find() = %+v, %v", got, ok)
	}
	got.Edits[0].New = "changed after finding"
	if again, _ := m.Find("k"); again.Edits[0].New != "jwt.RegisteredClaims" {
		t.Fatal("a found fix shares its edits with the memory")
	}
}

func TestACloneIsIndependent(t *testing.T) {
	m := &Memory{}
	if err := m.Save(sample("a", 5)); err != nil {
		t.Fatal(err)
	}
	c := m.Clone()
	if err := c.Save(sample("b", 7)); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find("a"); !ok || c.Len() != 2 {
		t.Fatal("the clone lost what the memory knew")
	}
	if _, ok := m.Find("b"); ok || m.Len() != 1 {
		t.Fatal("saving in the clone changed the original")
	}
	var none *Memory
	if _, ok := none.Find("a"); ok || none.Clone().Len() != 0 || none.Save(sample("a", 1)) == nil {
		t.Fatal("a nil memory must be empty and refuse to save")
	}
}

func TestDirKeepsFixesOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learned")
	d := &Dir{Path: path}
	if _, ok := d.Find("k"); ok {
		t.Fatal("an empty directory found a fix")
	}
	if err := d.Save(sample("k", 5)); err != nil {
		t.Fatal(err)
	}
	again := &Dir{Path: path}
	got, ok := again.Find("k")
	if !ok || got.PR != 5 || len(got.Edits) != 1 || got.Note != "Moves the claims to jwt.RegisteredClaims." || !got.At.Equal(sample("k", 5).At) {
		t.Fatalf("Find() = %+v, %v", got, ok)
	}
	if err := os.WriteFile(filepath.Join(path, Kind, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Find("broken"); ok {
		t.Fatal("a broken file was used as a fix")
	}
	if _, ok := again.Find("../k"); ok {
		t.Fatal("a key outside the directory was read")
	}
	if err := d.Save(Fix{}); err == nil {
		t.Fatal("a fix without a key was saved")
	}
	if err := (&Dir{}).Save(sample("k", 5)); err == nil {
		t.Fatal("a directory without a path saved a fix")
	}
}

func TestTwoPullRequestsWithTheSameKeyDoNotRace(t *testing.T) {
	for name, book := range map[string]Book{"memory": &Memory{}, "dir": &Dir{Path: filepath.Join(t.TempDir(), "learned")}} {
		t.Run(name, func(t *testing.T) {
			var wg sync.WaitGroup
			for pr := 1; pr <= 2; pr++ {
				wg.Go(func() {
					for i := range 20 {
						fix := sample("same", pr)
						fix.Summary = fmt.Sprintf("round %d", i)
						if err := book.Save(fix); err != nil {
							t.Error(err)
							return
						}
						if got, ok := book.Find("same"); !ok || len(got.Edits) != 1 {
							t.Errorf("Find() = %+v, %v", got, ok)
							return
						}
					}
				})
			}
			wg.Wait()
			got, ok := book.Find("same")
			if !ok || (got.PR != 1 && got.PR != 2) {
				t.Fatalf("Find() = %+v, %v", got, ok)
			}
		})
	}
}
