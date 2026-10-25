package tavily

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
)

var testKey = config.Secret(strings.Repeat("t", 40))

func TestSearchRequestAndResults(t *testing.T) {
	var auth, ctype, path, method string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ctype, path, method = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, `{"results":[
			{"title":"Migration guide","url":"https://github.com/golang-jwt/jwt/blob/main/MIGRATION_GUIDE.md","content":"short","raw_content":"# Guide\nlong text","score":0.92},
			{"title":"Releases","url":"https://github.com/golang-jwt/jwt/releases","content":"notes","raw_content":null,"score":0.85}]}`)
	}))
	defer srv.Close()
	c := New(testKey, "github.com", "pkg.go.dev")
	c.BaseURL = srv.URL + "/"
	c.HTTP = srv.Client()

	got, err := c.Search(t.Context(), "github.com/golang-jwt/jwt/v5 migration guide", 4)
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/search" || ctype != "application/json" || auth != "Bearer "+testKey.Reveal() {
		t.Fatalf("request: %s %s %s, auth ok: %v", method, path, ctype, auth == "Bearer "+testKey.Reveal())
	}
	if body["query"] != "github.com/golang-jwt/jwt/v5 migration guide" || body["max_results"] != float64(4) || body["search_depth"] != "basic" || body["include_raw_content"] != "markdown" {
		t.Fatalf("body = %v", body)
	}
	domains, _ := body["include_domains"].([]any)
	if len(domains) != 2 || domains[0] != "github.com" {
		t.Fatalf("domains = %v", body["include_domains"])
	}
	if len(got) != 2 || got[0].Score != 0.92 || got[0].RawContent != "# Guide\nlong text" || got[0].Content != "short" || got[1].RawContent != "" || got[1].URL != "https://github.com/golang-jwt/jwt/releases" {
		t.Fatalf("results = %+v", got)
	}
}

func TestSearchWithoutDomains(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, `{"results":[]}`)
	}))
	defer srv.Close()
	c := New(testKey)
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	got, err := c.Search(t.Context(), "golang.org/x/text changelog", 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("results %v error %v", got, err)
	}
	if _, present := body["include_domains"]; present || body["max_results"] != float64(1) {
		t.Fatalf("body = %v", body)
	}
}

func TestSearchErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"unauthorized": {401, `{"detail":{"error":"Unauthorized: missing or invalid API key."}}`, "Unauthorized"},
		"rate limited": {429, `{"detail":{"error":"Your request has been blocked due to excessive requests."}}`, "excessive requests"},
		"plain":        {500, `{"error":"boom"}`, "boom"},
		"message":      {400, `{"message":"bad query"}`, "bad query"},
		"text":         {502, `bad gateway`, "bad gateway"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		c := New(testKey)
		c.BaseURL, c.HTTP = srv.URL, srv.Client()
		_, err := c.Search(t.Context(), "query", 3)
		srv.Close()
		apiErr, ok := errors.AsType[*APIError](err)
		if !ok || apiErr.Status != tc.status || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

func TestSearchRefusesBadInput(t *testing.T) {
	if _, err := New("").Search(t.Context(), "query", 3); err == nil {
		t.Error("a search without a key was attempted")
	}
	if _, err := New(testKey).Search(t.Context(), "  ", 3); err == nil {
		t.Error("an empty query was accepted")
	}
}

func TestSearchBadResponseAndRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>") }))
	c := New(testKey)
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	if _, err := c.Search(t.Context(), "q", 1); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error = %v", err)
	}
	srv.Close()
	c.BaseURL = "http://" + testKey.Reveal() + ".invalid"
	_, err := c.Search(t.Context(), "q", 1)
	if err == nil || strings.Contains(err.Error(), testKey.Reveal()) {
		t.Errorf("a transport error must not show the key: %v", err)
	}
}
