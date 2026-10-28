package learn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
)

const Kind = "fix"

var ErrNoKey = errors.New("a learned fix needs a key")

type Fix struct {
	Key       string       `json:"key"`
	Repo      string       `json:"repo"`
	Module    string       `json:"module,omitempty"`
	From      string       `json:"from,omitempty"`
	To        string       `json:"to,omitempty"`
	Signature string       `json:"signature"`
	Edits     []fixer.Edit `json:"edits"`
	Patch     string       `json:"patch"`
	Summary   string       `json:"summary,omitempty"`
	Note      string       `json:"note,omitempty"`
	PR        int          `json:"pr"`
	Calls     int          `json:"calls"`
	Micros    int64        `json:"micros"`
	At        time.Time    `json:"at"`
}

func (f Fix) clone() Fix {
	f.Edits = slices.Clone(f.Edits)
	return f
}

type Book interface {
	Find(key string) (Fix, bool)
	Save(fix Fix) error
	Covers(scope string) bool
}

var (
	located  = regexp.MustCompile(`^(\S+\.go):\d+(?::\d+)?:\s*(.*)$`)
	position = regexp.MustCompile(`(\.go):\d+(?::\d+)?`)
)

func Signature(failure string) string {
	var found, rest []string
	for _, line := range strings.Split(model.Normalize(failure), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := located.FindStringSubmatch(line); m != nil {
			found = append(found, path.Clean(m[1])+": "+position.ReplaceAllString(strings.TrimSpace(m[2]), "$1"))
			continue
		}
		rest = append(rest, position.ReplaceAllString(line, "$1"))
	}
	lines := found
	if len(lines) == 0 {
		lines = rest
	}
	slices.Sort(lines)
	return strings.Join(slices.Compact(lines), "\n")
}

func major(version string) string {
	v, err := pull.ParseVersion(version)
	if err != nil {
		return version
	}
	return "v" + strconv.Itoa(v.Major)
}

func digest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func Scope(repo string, bump *pull.Bump) string {
	module, from, to := "", "", ""
	if bump != nil {
		module, from, to = bump.Module, major(bump.From), major(bump.To)
	}
	return digest(repo, module, from, to)
}

func Key(repo string, bump *pull.Bump, failure string) string {
	return Scope(repo, bump) + digest(Signature(failure))
}

type Memory struct {
	mu    sync.Mutex
	fixes map[string]Fix
}

func (m *Memory) Find(key string) (Fix, bool) {
	if m == nil {
		return Fix{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fix, ok := m.fixes[key]
	if !ok {
		return Fix{}, false
	}
	return fix.clone(), true
}

func (m *Memory) Save(fix Fix) error {
	if m == nil {
		return errors.New("there is no memory to save the fix in")
	}
	if fix.Key == "" {
		return ErrNoKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fixes == nil {
		m.fixes = map[string]Fix{}
	}
	m.fixes[fix.Key] = fix.clone()
	return nil
}

func (m *Memory) Covers(scope string) bool {
	if m == nil || scope == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, fix := range m.fixes {
		if strings.HasPrefix(key, scope) && len(fix.Edits) > 0 {
			return true
		}
	}
	return false
}

func (m *Memory) Clone() *Memory {
	out := &Memory{fixes: map[string]Fix{}}
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, fix := range m.fixes {
		out.fixes[key] = fix.clone()
	}
	return out
}

func (m *Memory) Len() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.fixes)
}

type Dir struct {
	Path string
	mu   sync.Mutex
}

func (d *Dir) store() fixture.Dir { return fixture.Dir{Path: d.Path} }

func (d *Dir) Find(key string) (Fix, bool) {
	if d == nil || d.Path == "" || key == "" {
		return Fix{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var fix Fix
	if err := d.store().Load(Kind, key, &fix); err != nil || fix.Key != key || len(fix.Edits) == 0 {
		return Fix{}, false
	}
	return fix, true
}

func (d *Dir) Covers(scope string) bool {
	if d == nil || d.Path == "" || scope == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(d.Path, Kind))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if name := e.Name(); e.Type().IsRegular() && strings.HasPrefix(name, scope) && strings.HasSuffix(name, ".json") {
			return true
		}
	}
	return false
}

func (d *Dir) Save(fix Fix) error {
	if d == nil || d.Path == "" {
		return errors.New("there is no directory to save the fix in")
	}
	if fix.Key == "" {
		return ErrNoKey
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.store().Save(Kind, fix.Key, fix)
}
