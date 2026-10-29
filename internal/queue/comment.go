package queue

import (
	"fmt"
	"strings"

	"github.com/aalvaropc/upkeep/internal/confidence"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/receipt"
)

func usd(micros int64) string {
	return fmt.Sprintf("$%.4f", float64(micros)/1_000_000)
}

func patchComment(res engine.Result) string {
	var b strings.Builder
	b.WriteString("Upkeep prepared a fix for this pull request.")
	if s := strings.TrimSpace(res.Summary); s != "" {
		b.WriteString(" " + s)
	}
	if res.PR.Fork {
		b.WriteString(" It cannot push to a branch in a fork, so here is how to apply it.\n")
	} else {
		b.WriteString(" It does not push to a person's branch without the `upkeep` label, so here is how to apply it.\n")
	}
	base := strings.TrimPrefix(res.PR.Base, "origin/")
	if base == "" {
		base = "main"
	}
	switch {
	case len(res.Resolved) > 0:
		files := map[string]bool{}
		for _, r := range res.Resolved {
			files[r.Path] = true
		}
		fmt.Fprintf(&b, "\nMerge `%s` into this branch. Git stops on %d conflicting %s in %d %s; Upkeep resolved %s like this, and every check passed on the result:\n", base, len(res.Resolved), plural(len(res.Resolved), "region", "regions"), len(files), plural(len(files), "file", "files"), plural(len(res.Resolved), "it", "them"))
		for _, r := range res.Resolved {
			fmt.Fprintf(&b, "\n`%s`\n\n```go\n%s\n```\n", r.Path, strings.TrimRight(r.Text, "\n"))
		}
	case res.MergedBase:
		fmt.Fprintf(&b, "\nMerge `%s` into this branch: it merges cleanly, and every check passed on the result.\n", base)
	}
	if patch := strings.TrimRight(res.Patch, "\n"); patch != "" {
		if res.MergedBase {
			b.WriteString("\nThen apply this change:\n\n")
		} else {
			b.WriteString("\nApply this change:\n\n")
		}
		b.WriteString("```diff\n" + patch + "\n```\n")
	}
	if len(res.Flaky) > 0 {
		fmt.Fprintf(&b, "\nTests that failed once and passed on a rerun, left alone: %s.\n", strings.Join(res.Flaky, ", "))
	}
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func diagnosisComment(res engine.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Upkeep could not finish this pull request: %s.\n", strings.TrimRight(res.Reason, "."))
	if text := strings.TrimSpace(res.Diagnosis); text != "" {
		b.WriteString("\n")
		b.WriteString(text)
		b.WriteString("\n")
	}
	return b.String()
}

func receiptComment(res engine.Result) string {
	if res.Receipt == nil {
		return ""
	}
	lines := res.Receipt.Lines()
	totals := res.Receipt.Totals()
	var b strings.Builder
	b.WriteString("Upkeep receipt\n\n")
	if s := strings.TrimSpace(res.Summary); s != "" {
		b.WriteString(s + "\n\n")
	}
	footer := confidenceLine(res.Confidence) + searchLine(res.Receipt.Searches())
	if l := res.Learned; l != nil {
		fmt.Fprintf(&b, "Learned fix: Upkeep reused the verified fix from #%d with 0 model calls; the first time cost %s.\n", l.From, usd(l.SavedMicros))
		if len(lines) == 0 {
			if footer != "" {
				b.WriteString("\n" + footer)
			}
			return b.String()
		}
		b.WriteString("\n")
	}
	if len(lines) == 0 {
		b.WriteString("No model was called; the checks were enough.\n")
		if footer != "" {
			b.WriteString("\n" + footer)
		}
		return b.String()
	}
	b.WriteString("| Model | Role | Calls | Prompt | Completion | Cost |\n|---|---|---|---|---|---|\n")
	for _, l := range lines {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %s |\n", l.Model, l.Role, l.Calls, l.Usage.PromptTokens, l.Usage.CompletionTokens, usd(l.Charge.Micros))
	}
	fmt.Fprintf(&b, "\nTotal %s. Attempts: %d.", usd(totals.Charge.Micros), res.Receipt.Attempts())
	if totals.Charge.BaselineMicros > 0 {
		fmt.Fprintf(&b, " The same tokens at the largest model's price would have cost %s; that is a price comparison, not a claim that it would have fixed it.", usd(totals.Charge.BaselineMicros))
	}
	b.WriteString("\n")
	if footer != "" {
		b.WriteString("\n" + footer)
	}
	return b.String()
}

func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func searchLine(searches []receipt.Search) string {
	if len(searches) == 0 {
		return ""
	}
	kept, failed, recorded := 0, 0, true
	var micros int64
	for _, s := range searches {
		kept += s.Kept
		micros += s.Micros
		recorded = recorded && s.Recorded
		if s.Error != "" {
			failed++
		}
	}
	name := forecast.ProviderLabel(searches[0].Provider)
	line := fmt.Sprintf("Web search: %s, %s kept", counted(len(searches), name+" search", name+" searches"), counted(kept, "page", "pages"))
	if failed > 0 {
		line += fmt.Sprintf(", %d failed", failed)
	}
	if micros > 0 {
		line += ", " + usd(micros)
	}
	if recorded {
		line += ", from the recorded run"
	}
	return line + ".\n"
}

func confidenceLine(a confidence.Assessment) string {
	if !a.Known() {
		return ""
	}
	return fmt.Sprintf("Merge confidence: %s (%s).\n", a.Level, strings.Join(a.Reasons, "; "))
}
