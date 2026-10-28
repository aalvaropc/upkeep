package confidence

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type Level string

const (
	High   Level = "high"
	Medium Level = "medium"
	Low    Level = "low"
)

type Input struct {
	Outcome   pull.Outcome
	Blocker   pull.Blocker
	Bump      *pull.Bump
	Attempts  int
	Learned   int
	Files     int
	Lines     int
	MaxLines  int
	Predicted int
	Confirmed int
	Flaky     []string
}

type Assessment struct {
	Level   Level    `json:"level"`
	Reasons []string `json:"reasons"`
	Risk    int      `json:"-"`
}

func (a Assessment) Known() bool { return a.Level != "" }

func Assess(in Input) Assessment {
	if in.Outcome != pull.Mergeable && in.Outcome != pull.Merged {
		return Assessment{}
	}
	var a Assessment
	written := in.Attempts > 0 || in.Learned > 0
	switch {
	case in.Learned > 0:
		a.note(0, fmt.Sprintf("reuses the verified fix learned from #%d", in.Learned))
	case in.Attempts > 1:
		a.note(in.Attempts-1, fmt.Sprintf("fixed on attempt %d", in.Attempts))
	case in.Attempts == 1:
		a.note(0, "fixed on the first attempt")
	case in.Blocker == pull.ConflictSmall:
		a.note(0, "a model resolved a small conflict")
	default:
		a.note(0, "no code written by a model")
	}
	if written && in.Lines > 0 {
		a.note(0, fmt.Sprintf("%s in %s", count(in.Lines, "line"), count(in.Files, "file")))
		if in.MaxLines > 0 && in.Lines*2 > in.MaxLines {
			a.note(1, fmt.Sprintf("more than half of the %d lines a fix may change", in.MaxLines))
		}
	}
	if in.Bump != nil && in.Bump.Level == pull.Major {
		a.note(1, "a major upgrade")
	}
	if in.Predicted > 0 {
		a.note(0, fmt.Sprintf("the build confirmed %d of %d predicted changes", in.Confirmed, in.Predicted))
	}
	if len(in.Flaky) > 0 {
		a.note(1, "a flaky test: "+strings.Join(in.Flaky, ", "))
	}
	a.Level = level(a.Risk)
	return a
}

func After(a Assessment, pr int, files []string) Assessment {
	if !a.Known() {
		return a
	}
	a.Reasons = slices.Clone(a.Reasons)
	a.note(1, fmt.Sprintf("conflicts with #%d in %s, which merges first", pr, strings.Join(files, ", ")))
	a.Level = level(a.Risk)
	return a
}

func (a *Assessment) note(risk int, reason string) {
	a.Risk += risk
	a.Reasons = append(a.Reasons, reason)
}

func level(risk int) Level {
	switch {
	case risk <= 0:
		return High
	case risk == 1:
		return Medium
	default:
		return Low
	}
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
