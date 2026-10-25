package github

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
)

func TestModuleRepo(t *testing.T) {
	cases := map[string]struct {
		owner, repo string
		ok          bool
	}{
		"github.com/golang-jwt/jwt/v5":               {"golang-jwt", "jwt", true},
		"github.com/golang-jwt/jwt":                  {"golang-jwt", "jwt", true},
		"github.com/example/multi-module/service/s3": {"example", "multi-module", true},
		"golang.org/x/text":                          {"", "", false},
		"github.com/onlyowner":                       {"", "", false},
		"gopkg.in/yaml.v3":                           {"", "", false},
		"":                                           {"", "", false},
	}
	for module, want := range cases {
		owner, repo, ok := ModuleRepo(module)
		if ok != want.ok || owner != want.owner || repo != want.repo {
			t.Errorf("ModuleRepo(%q) = %q %q %v", module, owner, repo, ok)
		}
	}
	for module, want := range map[string]string{"github.com/a/b/v5": "v5", "github.com/a/b": "", "github.com/a/b/v2x": "", "github.com/a/b/v10": "v10"} {
		if got := MajorSuffix(module); got != want {
			t.Errorf("MajorSuffix(%q) = %q, want %q", module, got, want)
		}
	}
}

func releaseList(from, count int, drafts bool) string {
	var items []string
	for i := 0; i < count; i++ {
		items = append(items, fmt.Sprintf(`{"tag_name":"v1.%d.0","name":"Release %d","body":"notes %d","html_url":"https://github.com/o/r/releases/tag/v1.%d.0","published_at":"2026-01-01T00:00:00Z","draft":%v}`, from+i, from+i, from+i, from+i, drafts && i == 0))
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestListReleasesPagesAndDrafts(t *testing.T) {
	var paths []string
	var auth, version string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		auth, version = r.Header.Get("Authorization"), r.Header.Get("X-GitHub-Api-Version")
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprint(w, releaseList(0, releasesPerPage, true))
		default:
			fmt.Fprint(w, releaseList(200, 5, false))
		}
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/", HTTP: srv.Client(), Token: config.Secret(strings.Repeat("g", 40))}
	releases, err := c.ListReleases(t.Context(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != releasesPerPage-1+5 {
		t.Fatalf("got %d releases, want the drafts left out", len(releases))
	}
	if len(paths) != 2 || paths[0] != "/repos/o/r/releases?per_page=100&page=1" || paths[1] != "/repos/o/r/releases?per_page=100&page=2" {
		t.Fatalf("paths = %v", paths)
	}
	if auth != "Bearer "+strings.Repeat("g", 40) || version != "2022-11-28" {
		t.Fatalf("headers: auth ok %v, version %q", auth == "Bearer "+strings.Repeat("g", 40), version)
	}
	if releases[0].TagName != "v1.1.0" || releases[0].Body != "notes 1" || releases[0].HTMLURL == "" || releases[0].PublishedAt.IsZero() {
		t.Fatalf("release = %+v", releases[0])
	}
}

func TestListReleasesWithoutAToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, "[]")
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if got, err := c.ListReleases(t.Context(), "o", "r"); err != nil || len(got) != 0 || auth != "" {
		t.Fatalf("releases %v error %v auth %q", got, err, auth)
	}
}

func TestListReleasesErrors(t *testing.T) {
	cases := map[string]struct {
		status  int
		limited bool
		body    string
		want    string
	}{
		"not found":    {404, false, `{"message":"Not Found"}`, "Not Found"},
		"rate limited": {403, true, `{"message":"API rate limit exceeded"}`, "rate limit reached"},
		"no body":      {500, false, ``, "HTTP 500"},
	}
	for name, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.limited {
				w.Header().Set("X-RateLimit-Remaining", "0")
			}
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
		_, err := c.ListReleases(t.Context(), "o", "r")
		srv.Close()
		ghErr, ok := errors.AsType[*Error](err)
		if !ok || ghErr.Status != tc.status || ghErr.RateLimited != tc.limited || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

func TestListReleasesBadJSONAndRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "nope") }))
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.ListReleases(t.Context(), "o", "r"); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error = %v", err)
	}
	srv.Close()
	token := config.Secret(strings.Repeat("g", 40))
	c = &Client{BaseURL: "http://" + token.Reveal() + ".invalid", Token: token}
	_, err := c.ListReleases(t.Context(), "o", "r")
	if err == nil || strings.Contains(err.Error(), token.Reveal()) {
		t.Errorf("a transport error must not show the token: %v", err)
	}
}
