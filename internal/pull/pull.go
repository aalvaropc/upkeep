package pull

import (
	"fmt"
	"strconv"
	"strings"
)

type AuthorKind string

const (
	Person AuthorKind = "person"
	Bot    AuthorKind = "bot"
)

type Author struct {
	Login string     `json:"login"`
	Kind  AuthorKind `json:"kind"`
}

type Kind string

const (
	Change     Kind = "change"
	Dependency Kind = "dependency"
)

type Level string

const (
	Patch Level = "patch"
	Minor Level = "minor"
	Major Level = "major"
)

type Bump struct {
	Module string `json:"module"`
	From   string `json:"from"`
	To     string `json:"to"`
	Level  Level  `json:"level"`
}

type Blocker string

const (
	Behind        Blocker = "behind"
	ConflictSmall Blocker = "conflict"
	ConflictLarge Blocker = "conflict_large"
	Build         Blocker = "build"
	Test          Blocker = "test"
	Flaky         Blocker = "flaky"
	Format        Blocker = "format"
	Clean         Blocker = "clean"
)

type Outcome string

const (
	Pending    Outcome = ""
	Merged     Outcome = "merged"
	Mergeable  Outcome = "mergeable"
	NeedsHuman Outcome = "needs_human"
	Failed     Outcome = "failed"
)

type Request struct {
	Number  int      `json:"number"`
	Title   string   `json:"title"`
	Author  Author   `json:"author"`
	Labels  []string `json:"labels,omitempty"`
	Base    string   `json:"base"`
	Branch  string   `json:"branch"`
	HeadSHA string   `json:"head_sha,omitempty"`
	Kind    Kind     `json:"kind"`
	Bump    *Bump    `json:"bump,omitempty"`
}

func (r Request) HasLabel(name string) bool {
	for _, l := range r.Labels {
		if strings.EqualFold(l, name) {
			return true
		}
	}
	return false
}

func LevelOf(from, to string) (Level, error) {
	a, err := parseVersion(from)
	if err != nil {
		return "", err
	}
	b, err := parseVersion(to)
	if err != nil {
		return "", err
	}
	switch {
	case a[0] != b[0]:
		return Major, nil
	case a[1] != b[1]:
		return Minor, nil
	default:
		return Patch, nil
	}
}

func parseVersion(v string) ([3]int, error) {
	var out [3]int
	s, ok := strings.CutPrefix(v, "v")
	if !ok {
		return out, fmt.Errorf("version %q: missing the v prefix", v)
	}
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("version %q: want vMAJOR.MINOR.PATCH", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, fmt.Errorf("version %q: %q is not a number", v, p)
		}
		out[i] = n
	}
	return out, nil
}
