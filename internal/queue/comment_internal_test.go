package queue

import (
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/receipt"
)

func TestAPatchCommentForAForkSaysItCannotPush(t *testing.T) {
	body := patchComment(engine.Result{PR: pull.Request{Fork: true}, Outcome: pull.Mergeable, Changed: true, WantsPatch: true, Patch: "diff --git a/x b/x\n+line\n"})
	if !strings.Contains(body, "It cannot push to a branch in a fork") || strings.Contains(body, "`upkeep` label") || !strings.Contains(body, "Apply this change:") {
		t.Fatalf("comment:\n%s", body)
	}
}

func TestTheReceiptCarriesTheForecastWithItsMarksAndSources(t *testing.T) {
	res := engine.Result{
		Forecast: &forecast.Forecast{
			Module: "github.com/golang-jwt/jwt/v5", From: "v4.5.2", To: "v5.3.1",
			Breaking: []forecast.Change{
				{Symbol: "jwt.StandardClaims", Description: "removed; use RegisteredClaims instead"},
				{Symbol: "jwt.Token", Description: "Signature field changed from string to []byte"},
				{Symbol: "jwt.Parse", Description: "returns a different error"},
				{Symbol: "x", Description: "too short to look for"},
			},
			Sources: []string{"https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md"},
			Search:  &forecast.SearchTrace{Kept: []string{"https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md", "https://pkg.go.dev/github.com/golang-jwt/jwt/v5"}},
		},
		Confirmed: []string{"jwt.StandardClaims"},
		FixedToo:  []string{"jwt.Parse"},
		Missed:    []string{"jwt.Token"},
		Receipt:   receipt.New(),
	}
	body := receiptComment(res)
	for _, want := range []string{
		"Forecast for github.com/golang-jwt/jwt/v5 v4.5.2 → v5.3.1:\n",
		"- `jwt.StandardClaims`: removed; use RegisteredClaims instead (confirmed by the build)\n",
		"- `jwt.Token`: Signature field changed from string to []byte (not seen in the build)\n",
		"- `jwt.Parse`: returns a different error (changed by the fix)\n",
		"- `x`: too short to look for\n",
		"Sources: https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md, https://pkg.go.dev/github.com/golang-jwt/jwt/v5\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("the receipt lacks %q:\n%s", want, body)
		}
	}
	if strings.Count(body, "MIGRATION_GUIDE.md") != 1 {
		t.Fatalf("a source must be listed once:\n%s", body)
	}
}

func TestAForecastWhoseBuildPassedSaysSo(t *testing.T) {
	res := engine.Result{
		Forecast:  &forecast.Forecast{Module: "example.com/m", From: "v1.0.0", To: "v1.1.0", Source: "api", Breaking: []forecast.Change{{Symbol: "m.Old", Description: "removed in v1.1.0"}}},
		Missed:    []string{"m.Old"},
		BuildFine: true,
		Receipt:   receipt.New(),
	}
	body := receiptComment(res)
	if !strings.Contains(body, "Forecast for example.com/m v1.0.0 → v1.1.0, from the module's exported API:\n") || !strings.Contains(body, "(not seen: the build passed)") || strings.Contains(body, "Sources:") {
		t.Fatalf("receipt:\n%s", body)
	}
}

func TestAReceiptWithoutAForecastHasNoForecastBlock(t *testing.T) {
	for _, f := range []*forecast.Forecast{nil, {Module: "example.com/m", From: "v1.0.0", To: "v1.1.0"}} {
		if body := receiptComment(engine.Result{Forecast: f, Receipt: receipt.New()}); strings.Contains(body, "Forecast") {
			t.Fatalf("receipt:\n%s", body)
		}
	}
}
