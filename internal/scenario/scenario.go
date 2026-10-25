package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/aalvaropc/upkeep/internal/pull"
)

type Expect struct {
	Blocker pull.Blocker `json:"blocker"`
	Outcome pull.Outcome `json:"outcome"`
	Pushed  bool         `json:"pushed,omitempty"`
	Patch   bool         `json:"patch,omitempty"`
}

type PR struct {
	Number int         `json:"number"`
	Title  string      `json:"title"`
	Branch string      `json:"branch"`
	Author pull.Author `json:"author"`
	Labels []string    `json:"labels,omitempty"`
	Bump   *pull.Bump  `json:"bump,omitempty"`
	Recipe string      `json:"recipe"`
	Expect Expect      `json:"expect"`
}

type Scenario struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Base       string `json:"base"`
	PRs        []PR   `json:"prs"`
}

func Load(r io.Reader) (Scenario, error) {
	var s Scenario
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Scenario{}, fmt.Errorf("scenario: %w", err)
	}
	if err := s.Validate(); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

var (
	blockers = []pull.Blocker{pull.Behind, pull.ConflictSmall, pull.ConflictLarge, pull.Build, pull.Test, pull.Flaky, pull.Format, pull.Clean}
	outcomes = []pull.Outcome{pull.Merged, pull.Mergeable, pull.NeedsHuman, pull.Failed}
)

func (s Scenario) Validate() error {
	var errs []error
	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, errors.New("scenario: name is required"))
	}
	if s.Base == "" {
		errs = append(errs, errors.New("scenario: base branch is required"))
	}
	if len(s.PRs) == 0 {
		errs = append(errs, errors.New("scenario: at least one pull request is required"))
	}
	numbers := map[int]bool{}
	branches := map[string]bool{}
	for _, pr := range s.PRs {
		where := fmt.Sprintf("scenario: pull request %d", pr.Number)
		switch {
		case pr.Number <= 0:
			errs = append(errs, fmt.Errorf("%s: number must be positive", where))
		case numbers[pr.Number]:
			errs = append(errs, fmt.Errorf("%s: number is used twice", where))
		}
		numbers[pr.Number] = true
		switch {
		case pr.Branch == "" || pr.Branch == s.Base:
			errs = append(errs, fmt.Errorf("%s: needs its own branch", where))
		case branches[pr.Branch]:
			errs = append(errs, fmt.Errorf("%s: branch %q is used twice", where, pr.Branch))
		}
		branches[pr.Branch] = true
		if strings.TrimSpace(pr.Title) == "" {
			errs = append(errs, fmt.Errorf("%s: title is required", where))
		}
		if pr.Author.Login == "" || (pr.Author.Kind != pull.Person && pr.Author.Kind != pull.Bot) {
			errs = append(errs, fmt.Errorf("%s: author needs a login and a kind of person or bot", where))
		}
		if pr.Recipe == "" {
			errs = append(errs, fmt.Errorf("%s: recipe is required", where))
		}
		if !slices.Contains(blockers, pr.Expect.Blocker) {
			errs = append(errs, fmt.Errorf("%s: unknown expected blocker %q", where, pr.Expect.Blocker))
		}
		if !slices.Contains(outcomes, pr.Expect.Outcome) {
			errs = append(errs, fmt.Errorf("%s: unknown expected outcome %q", where, pr.Expect.Outcome))
		}
		if pr.Expect.Pushed && pr.Expect.Patch {
			errs = append(errs, fmt.Errorf("%s: a fix is either pushed or attached as a patch, not both", where))
		}
		if pr.Expect.Outcome == pull.Merged && pr.Author.Kind != pull.Bot {
			errs = append(errs, fmt.Errorf("%s: only a bot's pull request can be expected to merge", where))
		}
	}
	return errors.Join(errs...)
}
