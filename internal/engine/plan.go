package engine

import (
	"fmt"

	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/verify"
)

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func approach(blocker pull.Blocker, pol policy.Policy, base string) string {
	checks := plural(len(verify.Verify(verify.Input{Policy: pol})), "check")
	repair := fmt.Sprintf("Super repairs it with %s, up to %s, then %s", plural(len(fixer.Tools()), "tool"), plural(pol.MaxAttempts, "attempt"), checks)
	switch blocker {
	case pull.Clean:
		return "nothing blocks it: run the " + checks
	case pull.Flaky:
		return "a flaky test: report it and change nothing, then " + checks
	case pull.Behind:
		return fmt.Sprintf("behind %s: merge %s mechanically, then %s", base, base, checks)
	case pull.Format:
		return "unformatted files: gofmt them, then " + checks
	case pull.ConflictSmall:
		return "a small conflict: Ultra resolves it in one call, then " + checks
	case pull.ConflictLarge:
		return "a large conflict: Ultra writes a diagnosis for a person"
	case pull.Build:
		return "build fails: " + repair
	case pull.Test:
		return "tests fail: " + repair
	case pull.Unsupported:
		return "not a Go module: no model is asked, and a person decides"
	case pull.NestedModule:
		return "a module in a subdirectory: no model is asked, and a person decides"
	default:
		return string(blocker)
	}
}
