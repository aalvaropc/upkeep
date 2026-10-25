package fixer

import (
	"os"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/sandbox"
)

func TestLiveSuperRepairsARename(t *testing.T) {
	requireGo(t)
	if os.Getenv("UPKEEP_LIVE_TESTS") != "1" {
		t.Skip("set UPKEEP_LIVE_TESTS=1 and NEBIUS_API_KEY to call Token Factory")
	}
	cfg, err := config.Load(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NebiusAPIKey.IsSet() {
		t.Skip("NEBIUS_API_KEY is not set")
	}
	tf := model.NewTokenFactory(cfg.NebiusBaseURL, cfg.NebiusAPIKey)
	book, err := tf.FetchPrices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	f := Fixer{
		Model:     tf,
		ModelName: cfg.Models.Super,
		Meter:     model.Meter{Prices: book, Baseline: cfg.Models.Ultra},
		Runner:    sandbox.GoRunner{Executor: sandbox.Exec{}},
		Executor:  sandbox.Exec{},
		Deadline:  3 * time.Minute,
	}
	dir := renameModule(t)
	prior := ""
	for attempt := 1; attempt <= 3; attempt++ {
		a := f.Run(t.Context(), Task{Dir: dir, Failure: failureOf(t, dir), Prior: prior})
		t.Logf("attempt %d: success %v, %s, %d turns, %d checks, %+v tokens, %d micro-USD (%d at the baseline), %d edits",
			attempt, a.Success, a.Stop, a.Turns, a.Checks, a.Usage, a.Charge.Micros, a.Charge.BaselineMicros, len(a.Edits))
		if a.Err != nil {
			t.Fatal(a.Err)
		}
		if a.Success {
			return
		}
		prior = a.Failure
	}
	t.Fatal("Super did not repair the module in three attempts")
}
