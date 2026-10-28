package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
)

type fixed struct{ results map[int]engine.Result }

func (f fixed) Process(_ context.Context, in engine.Input) engine.Result {
	res := f.results[in.PR.Number]
	res.PR = in.PR
	return res
}

func newDemoServer(t *testing.T, maxSessions int, trust bool) *httptest.Server {
	t.Helper()
	template := forgetest.New(t)
	hub := events.NewHub(200)
	proc := fixed{results: map[int]engine.Result{
		1: {Outcome: pull.Mergeable, Blocker: pull.Behind},
		3: {Outcome: pull.NeedsHuman, Blocker: pull.ConflictLarge, Reason: "too big", Diagnosis: "both sides rewrote it"},
	}}
	m, err := demo.NewManager(t.Context(), demo.Options{
		Scenario:    forgetest.Scenario(),
		Template:    template.Repo,
		Root:        filepath.Join(t.TempDir(), "sessions"),
		Git:         gitx.Git{},
		Hub:         hub,
		Processor:   func(queue.Emitter, learn.Book) (queue.Processor, error) { return proc, nil },
		Limits:      engine.Limits{PRsPerRun: 2, RunsPerServer: 2},
		MaxSessions: maxSessions,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	srv := httptest.NewServer(New(Options{
		Hub: hub, Version: "v0.0.1-test", Keepalive: time.Minute, UI: builtUI,
		Demo: m, TrustProxy: trust, Info: func() map[string]any { return map[string]any{"mode": "replay"} },
	}))
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, method, url string, header http.Header) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func decodeBody[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(r.body), &v); err != nil {
		t.Fatalf("decoding %q: %v", r.body, err)
	}
	return v
}

func TestTheWholeDemoFlowOverHTTP(t *testing.T) {
	srv := newDemoServer(t, 10, false)
	created := call(t, http.MethodPost, srv.URL+"/api/sessions", nil)
	if created.status != http.StatusCreated || created.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create: %d %s", created.status, created.body)
	}
	snap := decodeBody[demo.Snapshot](t, created)
	if snap.ID == "" || snap.Status != demo.Idle || len(snap.PRs) != 4 {
		t.Fatalf("created snapshot = %+v", snap)
	}
	if created.header.Get("Location") != "/api/sessions/"+snap.ID {
		t.Fatalf("Location = %q", created.header.Get("Location"))
	}
	base := srv.URL + "/api/sessions/" + snap.ID

	if r := call(t, http.MethodPost, base+"/run", nil); r.status != http.StatusAccepted {
		t.Fatalf("run: %d %s", r.status, r.body)
	}
	if r := call(t, http.MethodPost, base+"/run", nil); r.status != http.StatusConflict {
		t.Fatalf("a second run: %d %s", r.status, r.body)
	}
	deadline := time.Now().Add(10 * time.Second)
	var finished demo.Snapshot
	for time.Now().Before(deadline) {
		finished = decodeBody[demo.Snapshot](t, call(t, http.MethodGet, base, nil))
		if finished.Status == demo.Finished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if finished.Status != demo.Finished || finished.PRs[2].Outcome != pull.NeedsHuman {
		t.Fatalf("finished snapshot = %+v", finished)
	}

	detail := call(t, http.MethodGet, base+"/prs/3", nil)
	if detail.status != http.StatusOK || !strings.Contains(detail.body, "both sides rewrote it") {
		t.Fatalf("detail: %d %s", detail.status, detail.body)
	}
	if r := call(t, http.MethodGet, base+"/prs/99", nil); r.status != http.StatusNotFound {
		t.Fatalf("an unknown pull request: %d", r.status)
	}
	if r := call(t, http.MethodGet, base+"/prs/abc", nil); r.status != http.StatusBadRequest {
		t.Fatalf("a malformed number: %d", r.status)
	}

	if r := call(t, http.MethodDelete, base, nil); r.status != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.status, r.body)
	}
	if r := call(t, http.MethodGet, base, nil); r.status != http.StatusNotFound {
		t.Fatalf("a deleted session: %d", r.status)
	}
	if r := call(t, http.MethodPost, srv.URL+"/api/sessions/nope/run", nil); r.status != http.StatusNotFound {
		t.Fatalf("run on an unknown session: %d", r.status)
	}
}

func TestInfoReportsTheModeAndTheSessionCount(t *testing.T) {
	srv := newDemoServer(t, 10, false)
	call(t, http.MethodPost, srv.URL+"/api/sessions", nil)
	info := decodeBody[map[string]any](t, call(t, http.MethodGet, srv.URL+"/api/info", nil))
	if info["mode"] != "replay" || info["sessions"] != float64(1) {
		t.Fatalf("info = %v", info)
	}
}

func TestAFullDemoAsksVisitorsToComeBack(t *testing.T) {
	srv := newDemoServer(t, 1, false)
	first := decodeBody[demo.Snapshot](t, call(t, http.MethodPost, srv.URL+"/api/sessions", nil))
	if r := call(t, http.MethodPost, srv.URL+"/api/sessions/"+first.ID+"/run", nil); r.status != http.StatusAccepted {
		t.Fatalf("run: %d", r.status)
	}
	full := call(t, http.MethodPost, srv.URL+"/api/sessions", nil)
	if full.status != http.StatusCreated && full.status != http.StatusServiceUnavailable {
		t.Fatalf("a second session: %d %s", full.status, full.body)
	}
}

func TestSessionCreationIsRateLimitedPerClient(t *testing.T) {
	srv := newDemoServer(t, 200, true)
	from := func(ip string) response {
		return call(t, http.MethodPost, srv.URL+"/api/sessions", http.Header{"X-Forwarded-For": {ip + ", 10.0.0.1"}})
	}
	for i := range createPerMin {
		if r := from("203.0.113.7"); r.status != http.StatusCreated {
			t.Fatalf("request %d: %d %s", i+1, r.status, r.body)
		}
	}
	limited := from("203.0.113.7")
	if limited.status != http.StatusTooManyRequests || limited.header.Get("Retry-After") != "60" {
		t.Fatalf("the request over the limit: %d %v", limited.status, limited.header)
	}
	if r := from("203.0.113.8"); r.status != http.StatusCreated {
		t.Fatalf("another client was limited too: %d", r.status)
	}
}

func TestForwardedAddressesAreIgnoredUnlessTheProxyIsTrusted(t *testing.T) {
	srv := newDemoServer(t, 200, false)
	for i := range createPerMin {
		r := call(t, http.MethodPost, srv.URL+"/api/sessions", http.Header{"X-Forwarded-For": {"198.51.100." + string(rune('1'+i))}})
		if r.status != http.StatusCreated {
			t.Fatalf("request %d: %d", i+1, r.status)
		}
	}
	if r := call(t, http.MethodPost, srv.URL+"/api/sessions", http.Header{"X-Forwarded-For": {"198.51.100.99"}}); r.status != http.StatusTooManyRequests {
		t.Fatalf("a spoofed address escaped the limit: %d", r.status)
	}
}

func TestLimiterForgetsOldHits(t *testing.T) {
	l := newLimiter(2, time.Minute)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	var got []bool
	for range 3 {
		got = append(got, l.allow("a"))
	}
	if got[0] != true || got[1] != true || got[2] != false {
		t.Fatalf("three hits in a row were allowed as %v, want true, true, false", got)
	}
	now = now.Add(61 * time.Second)
	if !l.allow("a") {
		t.Fatal("a hit after the window should pass")
	}
}
