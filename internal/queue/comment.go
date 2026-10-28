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
	b.WriteString(" It does not push to a person's branch without the `upkeep` label, so here is the change as a patch:\n\n```diff\n")
	b.WriteString(strings.TrimRight(res.Patch, "\n"))
	b.WriteString("\n```\n")
	if len(res.Flaky) > 0 {
		fmt.Fprintf(&b, "\nTests that failed once and passed on a rerun, left alone: %s.\n", strings.Join(res.Flaky, ", "))
	}
	return b.String()
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
