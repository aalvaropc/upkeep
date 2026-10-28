package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixer"
	"github.com/aalvaropc/upkeep/internal/forge/forgetest"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/pull"
	"github.com/aalvaropc/upkeep/internal/queue"
)

type learner struct{ book learn.Book }

func (l learner) Process(_ context.Context, in engine.Input) engine.Result {
	res := engine.Result{PR: in.PR, Outcome: pull.Mergeable, Blocker: pull.Build, Summary: "learned it"}
	key := "rename-" + strconv.Itoa(in.PR.Number)
	if fix, ok := l.book.Find(key); ok {
		res.Summary = "reused it"
		res.Learned = &engine.Learned{From: fix.PR, SavedMicros: fix.Micros}
		return res
	}
	_ = l.book.Save(learn.Fix{Key: key, PR: in.PR.Number, Micros: 6400, Edits: []fixer.Edit{{Path: "a.go", Old: "Add", New: "Sum"}}})
	return res
}

func post(t *testing.T, url, body string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(data)}
}

func TestANewSessionCanInheritTheFixesOfAnother(t *testing.T) {
	template := forgetest.New(t)
	hub := events.NewHub(200)
	m, err := demo.NewManager(t.Context(), demo.Options{
		Scenario:  forgetest.Scenario(),
		Template:  template.Repo,
		Root:      filepath.Join(t.TempDir(), "sessions"),
		Git:       gitx.Git{},
		Hub:       hub,
		Processor: func(_ queue.Emitter, book learn.Book) (queue.Processor, error) { return learner{book: book}, nil },
		Limits:    engine.Limits{PRsPerRun: 2, RunsPerServer: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	srv := httptest.NewServer(New(Options{Hub: hub, Version: "v0.0.1-test", Keepalive: time.Minute, UI: builtUI, Demo: m}))
	t.Cleanup(srv.Close)

	summary := func(body string) (string, demo.Detail) {
		t.Helper()
		created := post(t, srv.URL+"/api/sessions", body)
		if created.status != http.StatusCreated {
			t.Fatalf("create with %q: %d %s", body, created.status, created.body)
		}
		id := decodeBody[demo.Snapshot](t, created).ID
		base := srv.URL + "/api/sessions/" + id
		if r := call(t, http.MethodPost, base+"/run", nil); r.status != http.StatusAccepted {
			t.Fatalf("run: %d %s", r.status, r.body)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if decodeBody[demo.Snapshot](t, call(t, http.MethodGet, base, nil)).Status == demo.Finished {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		return id, decodeBody[demo.Detail](t, call(t, http.MethodGet, base+"/prs/1", nil))
	}

	first, d := summary("")
	if d.Summary != "learned it" || d.Learned != nil {
		t.Fatalf("first session detail = %+v", d)
	}
	_, d = summary(`{"inherit":"` + first + `"}`)
	if d.Summary != "reused it" || d.Learned == nil || d.Learned.SavedMicros != 6400 {
		t.Fatalf("an heir did not reuse the fix: %+v", d)
	}
	for _, body := range []string{"  ", "{}", `{"inherit":"no-such-session"}`} {
		if _, d := summary(body); d.Summary != "learned it" {
			t.Fatalf("a session created with %q reused a fix it never learned", body)
		}
	}

	if r := post(t, srv.URL+"/api/sessions", "not json"); r.status != http.StatusBadRequest || !strings.Contains(r.body, "inherit") {
		t.Fatalf("a malformed body: %d %s", r.status, r.body)
	}
	if r := post(t, srv.URL+"/api/sessions", `{"inherit":"`+strings.Repeat("a", 2000)+`"}`); r.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("a body over the limit: %d %s", r.status, r.body)
	}
}
