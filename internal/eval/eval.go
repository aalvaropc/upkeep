package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

type Case struct {
	Run            int          `json:"run"`
	Number         int          `json:"number"`
	Title          string       `json:"title"`
	Blocker        pull.Blocker `json:"blocker"`
	ExpectBlocker  pull.Blocker `json:"expect_blocker"`
	Outcome        pull.Outcome `json:"outcome"`
	ExpectOutcome  pull.Outcome `json:"expect_outcome"`
	Pushed         bool         `json:"pushed"`
	ExpectPushed   bool         `json:"expect_pushed"`
	Patched        bool         `json:"patched"`
	ExpectPatch    bool         `json:"expect_patch"`
	Attempts       int          `json:"attempts"`
	Calls          int          `json:"calls"`
	PromptTokens   int          `json:"prompt_tokens"`
	OutputTokens   int          `json:"completion_tokens"`
	Micros         int64        `json:"micros"`
	BaselineMicros int64        `json:"baseline_micros"`
	DurationMS     int64        `json:"duration_ms"`
	OK             bool         `json:"ok"`
	Problems       []string     `json:"problems,omitempty"`
}

type Report struct {
	Scenario string    `json:"scenario"`
	Mode     string    `json:"mode"`
	Version  string    `json:"version"`
	Runs     int       `json:"runs"`
	At       time.Time `json:"at"`
	Notices  []string  `json:"notices,omitempty"`
	Degraded bool      `json:"degraded,omitempty"`
	Cases    []Case    `json:"cases"`
}

func did(it queue.Item, kind string) bool {
	for _, a := range it.Actions {
		if a.Kind == kind && a.Status == "done" {
			return true
		}
	}
	return false
}

func Check(run int, pr scenario.PR, it queue.Item) Case {
	res := it.Result
	c := Case{
		Run: run, Number: pr.Number, Title: pr.Title,
		Blocker: res.Blocker, ExpectBlocker: pr.Expect.Blocker,
		Outcome: it.Outcome, ExpectOutcome: pr.Expect.Outcome,
		Pushed: did(it, "push"), ExpectPushed: pr.Expect.Pushed,
		Patched: did(it, "patch"), ExpectPatch: pr.Expect.Patch,
		Attempts: res.Attempts, DurationMS: it.Duration.Milliseconds(),
	}
	if res.Receipt != nil {
		t := res.Receipt.Totals()
		c.Calls, c.PromptTokens, c.OutputTokens = t.Calls, t.Usage.PromptTokens, t.Usage.CompletionTokens
		c.Micros, c.BaselineMicros = t.Charge.Micros, t.Charge.BaselineMicros
	}
	if c.Blocker != c.ExpectBlocker {
		c.Problems = append(c.Problems, fmt.Sprintf("blocker %q, expected %q", c.Blocker, c.ExpectBlocker))
	}
	if c.Outcome != c.ExpectOutcome {
		why := ""
		if res.Reason != "" {
			why = " (" + res.Reason + ")"
		}
		c.Problems = append(c.Problems, fmt.Sprintf("outcome %q, expected %q%s", c.Outcome, c.ExpectOutcome, why))
	}
	if c.Pushed != c.ExpectPushed {
		c.Problems = append(c.Problems, fmt.Sprintf("pushed %v, expected %v", c.Pushed, c.ExpectPushed))
	}
	if c.Patched != c.ExpectPatch {
		c.Problems = append(c.Problems, fmt.Sprintf("patch attached %v, expected %v", c.Patched, c.ExpectPatch))
	}
	c.OK = len(c.Problems) == 0
	return c
}

func Run(ctx context.Context, m *demo.Manager, s scenario.Scenario, runs int, mode, version string) (Report, error) {
	report := Report{Scenario: s.Name, Mode: mode, Version: version, Runs: runs, At: time.Now().UTC()}
	byNumber := map[int]scenario.PR{}
	for _, pr := range s.PRs {
		byNumber[pr.Number] = pr
	}
	for run := 1; run <= runs; run++ {
		session, err := m.New(ctx, "")
		if err != nil {
			return report, err
		}
		if _, err := m.Start(session.ID); err != nil {
			return report, err
		}
		if err := session.Wait(ctx); err != nil {
			return report, err
		}
		for _, it := range session.Items() {
			report.Cases = append(report.Cases, Check(run, byNumber[it.PR.Number], it))
		}
		for _, n := range session.Notices() {
			if !slices.Contains(report.Notices, n) {
				report.Notices = append(report.Notices, n)
			}
			report.Degraded = report.Degraded || mode != "replay"
		}
		if err := m.Discard(session.ID); err != nil {
			return report, err
		}
	}
	return report, nil
}

type Row struct {
	Number         int
	Title          string
	Runs           int
	Passed         int
	MeanAttempts   float64
	MeanCalls      float64
	MeanMicros     float64
	MeanBaseline   float64
	MeanDurationMS float64
	Problems       []string
}

type Summary struct {
	Rows           []Row
	Cases          int
	Passed         int
	Micros         int64
	BaselineMicros int64
}

func (r Report) Summary() Summary {
	var s Summary
	byNumber := map[int]*Row{}
	for _, c := range r.Cases {
		row := byNumber[c.Number]
		if row == nil {
			row = &Row{Number: c.Number, Title: c.Title}
			byNumber[c.Number] = row
		}
		row.Runs++
		if c.OK {
			row.Passed++
		}
		row.MeanAttempts += float64(c.Attempts)
		row.MeanCalls += float64(c.Calls)
		row.MeanMicros += float64(c.Micros)
		row.MeanBaseline += float64(c.BaselineMicros)
		row.MeanDurationMS += float64(c.DurationMS)
		for _, p := range c.Problems {
			if !slices.Contains(row.Problems, p) {
				row.Problems = append(row.Problems, p)
			}
		}
		s.Cases++
		if c.OK {
			s.Passed++
		}
		s.Micros += c.Micros
		s.BaselineMicros += c.BaselineMicros
	}
	for _, row := range byNumber {
		n := float64(row.Runs)
		row.MeanAttempts /= n
		row.MeanCalls /= n
		row.MeanMicros /= n
		row.MeanBaseline /= n
		row.MeanDurationMS /= n
		s.Rows = append(s.Rows, *row)
	}
	slices.SortFunc(s.Rows, func(a, b Row) int { return a.Number - b.Number })
	return s
}

func (r Report) AllPassed() bool {
	return len(r.Cases) > 0 && !r.Degraded && r.Summary().Passed == len(r.Cases)
}

func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func usd(micros float64) string {
	return fmt.Sprintf("$%.4f", micros/1_000_000)
}

func (r Report) Markdown() string {
	s := r.Summary()
	var b strings.Builder
	fmt.Fprintf(&b, "# Evaluation: %s\n\n", r.Scenario)
	fmt.Fprintf(&b, "Mode `%s`, %d run(s), version `%s`, %s.\n\n", r.Mode, r.Runs, r.Version, r.At.Format("2006-01-02 15:04 UTC"))
	fmt.Fprintf(&b, "**%d of %d cases ended as expected.**", s.Passed, s.Cases)
	if s.Cases > 0 && s.BaselineMicros > 0 {
		perRun := float64(max(r.Runs, 1))
		fmt.Fprintf(&b, " The %d pull requests cost %s per run; the same tokens at the largest model's price would have cost %s (%.1f times as much). That is a price comparison, not a claim that the largest model would have fixed them.", len(s.Rows), usd(float64(s.Micros)/perRun), usd(float64(s.BaselineMicros)/perRun), float64(s.BaselineMicros)/float64(max(s.Micros, 1)))
	}
	b.WriteString("\n\n| # | Pull request | Passed | Attempts | Model calls | Cost | At the largest price | Seconds |\n|---|---|---|---|---|---|---|---|\n")
	for _, row := range s.Rows {
		fmt.Fprintf(&b, "| %d | %s | %d/%d | %.1f | %.1f | %s | %s | %.1f |\n", row.Number, row.Title, row.Passed, row.Runs, row.MeanAttempts, row.MeanCalls, usd(row.MeanMicros), usd(row.MeanBaseline), row.MeanDurationMS/1000)
	}
	for _, n := range r.Notices {
		fmt.Fprintf(&b, "\nNotice from the server: %s\n", n)
	}
	if r.Degraded {
		b.WriteString("\nThis evaluation did not run the way its mode says, so it does not count as a pass.\n")
	}
	var problems []string
	for _, row := range s.Rows {
		for _, p := range row.Problems {
			problems = append(problems, fmt.Sprintf("- #%d: %s", row.Number, p))
		}
	}
	if len(problems) > 0 {
		b.WriteString("\nDifferences from what the scenario expects:\n\n" + strings.Join(problems, "\n") + "\n")
	}
	return b.String()
}
