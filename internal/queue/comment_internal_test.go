package queue

import (
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/pull"
)

func TestAPatchCommentForAForkSaysItCannotPush(t *testing.T) {
	body := patchComment(engine.Result{PR: pull.Request{Fork: true}, Outcome: pull.Mergeable, Changed: true, WantsPatch: true, Patch: "diff --git a/x b/x\n+line\n"})
	if !strings.Contains(body, "It cannot push to a branch in a fork") || strings.Contains(body, "`upkeep` label") || !strings.Contains(body, "Apply this change:") {
		t.Fatalf("comment:\n%s", body)
	}
}
