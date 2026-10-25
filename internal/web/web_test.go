package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
)

func newServer(t *testing.T, keepalive time.Duration) (*httptest.Server, *events.Ring, chan struct{}) {
	t.Helper()
	hub := events.NewHub(100)
	ring := hub.Open("s1")
	done := make(chan struct{})
	srv := httptest.NewServer(New(Options{Hub: hub, Version: "v0.0.1-test", Keepalive: keepalive, Done: done}))
	t.Cleanup(srv.Close)
	return srv, ring, done
}

type response struct {
	status int
	header http.Header
	body   string
}

func get(t *testing.T, url string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
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

func TestHealthz(t *testing.T) {
	srv, _, _ := newServer(t, time.Minute)
	r := get(t, srv.URL+"/healthz")
	if r.status != http.StatusOK {
		t.Fatalf("status = %d", r.status)
	}
	body := r.body
	var got map[string]string
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "ok" || got["version"] != "v0.0.1-test" {
		t.Fatalf("healthz = %v", got)
	}
}

func TestBoardAndAssets(t *testing.T) {
	srv, _, _ := newServer(t, time.Minute)
	page := get(t, srv.URL+"/")
	if page.status != http.StatusOK || !strings.HasPrefix(page.header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET /: %d %s", page.status, page.header.Get("Content-Type"))
	}
	for _, want := range []string{`id="board"`, "Work the queue", `src="app.js"`} {
		if !strings.Contains(page.body, want) {
			t.Errorf("board page does not contain %q", want)
		}
	}
	if csp := page.header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
	if page.header.Get("X-Content-Type-Options") != "nosniff" || page.header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("security headers = %v", page.header)
	}
	for path, ctype := range map[string]string{"/app.js": "text/javascript", "/app.css": "text/css"} {
		asset := get(t, srv.URL+path)
		if asset.status != http.StatusOK || !strings.HasPrefix(asset.header.Get("Content-Type"), ctype) {
			t.Errorf("GET %s: %d %s", path, asset.status, asset.header.Get("Content-Type"))
		}
	}
	if missing := get(t, srv.URL+"/missing.js"); missing.status != http.StatusNotFound {
		t.Errorf("GET /missing.js: %d", missing.status)
	}
}

type stream struct {
	t      *testing.T
	resp   *http.Response
	lines  chan string
	cancel context.CancelFunc
}

func openStream(t *testing.T, url string, header http.Header) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(req.Header, header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	s := &stream{t: t, resp: resp, lines: make(chan string, 100), cancel: cancel}
	go func() {
		defer close(s.lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			s.lines <- sc.Text()
		}
	}()
	t.Cleanup(func() {
		cancel()
		resp.Body.Close()
	})
	return s
}

func (s *stream) frame() map[string]string {
	s.t.Helper()
	frame := map[string]string{}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-s.lines:
			if !ok {
				s.t.Fatal("stream closed while waiting for a frame")
			}
			if line == "" {
				if len(frame) > 0 {
					return frame
				}
				continue
			}
			k, v, _ := strings.Cut(line, ": ")
			frame[k] = v
		case <-timeout:
			s.t.Fatal("timed out waiting for a frame")
		}
	}
}

func TestStream(t *testing.T) {
	srv, ring, _ := newServer(t, 50*time.Millisecond)
	s := openStream(t, srv.URL+"/api/sessions/s1/events", nil)
	if ct := s.resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	hello := s.frame()
	if hello["event"] != "hello" || hello["retry"] != "3000" || hello["id"] != "" {
		t.Fatalf("first frame = %v", hello)
	}
	if !strings.Contains(hello["data"], `"session":"s1"`) {
		t.Fatalf("hello data = %s", hello["data"])
	}

	ring.Append(events.Event{Kind: events.RunStarted, Run: "r1"})
	got := s.frame()
	for got["event"] == "ping" {
		got = s.frame()
	}
	if got["id"] != "1" || got["event"] != "" {
		t.Fatalf("event frame = %v", got)
	}
	var e events.Event
	if err := json.Unmarshal([]byte(got["data"]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Kind != events.RunStarted || e.Run != "r1" || e.Seq != 1 {
		t.Fatalf("event = %+v", e)
	}

	ping := s.frame()
	if ping["event"] != "ping" || ping["id"] != "" {
		t.Fatalf("expected a keepalive ping, got %v", ping)
	}
}

func TestStreamResumes(t *testing.T) {
	srv, ring, _ := newServer(t, time.Minute)
	ring.Append(events.Event{Kind: events.RunStarted})
	ring.Append(events.Event{Kind: events.PRStep, PR: 3})

	byQuery := openStream(t, srv.URL+"/api/sessions/s1/events?after=1", nil)
	byQuery.frame()
	if f := byQuery.frame(); f["id"] != "2" {
		t.Fatalf("resume with after=1: first event id = %q", f["id"])
	}

	byHeader := openStream(t, srv.URL+"/api/sessions/s1/events?after=0", http.Header{"Last-Event-Id": {"1"}})
	byHeader.frame()
	if f := byHeader.frame(); f["id"] != "2" {
		t.Fatalf("resume with Last-Event-ID: first event id = %q", f["id"])
	}

	gap := openStream(t, srv.URL+"/api/sessions/s1/events?after=99", nil)
	gap.frame()
	if f := gap.frame(); f["event"] != "gap" {
		t.Fatalf("resume past the end: frame = %v, want a gap", f)
	}
	if f := gap.frame(); f["id"] != "1" {
		t.Fatalf("after the gap: first event id = %q", f["id"])
	}
}

func TestStreamErrors(t *testing.T) {
	srv, _, _ := newServer(t, time.Minute)
	if r := get(t, srv.URL+"/api/sessions/nope/events"); r.status != http.StatusNotFound {
		t.Fatalf("unknown session: %d", r.status)
	}
	if r := get(t, srv.URL+"/api/sessions/s1/events?after=-1"); r.status != http.StatusBadRequest {
		t.Fatalf("invalid resume point: %d", r.status)
	}
}

func TestStreamEndsOnShutdown(t *testing.T) {
	srv, _, done := newServer(t, time.Minute)
	s := openStream(t, srv.URL+"/api/sessions/s1/events", nil)
	s.frame()
	close(done)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-s.lines:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("stream did not end after shutdown")
		}
	}
}
