package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aalvaropc/upkeep/internal/config"
)

type call struct {
	method string
	path   string
	body   string
	auth   string
}

func fakeAPI(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body string)) (*Client, *[]call) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		calls = append(calls, call{method: r.Method, path: r.URL.RequestURI(), body: string(raw), auth: r.Header.Get("Authorization")})
		handle(w, r, string(raw))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: config.Secret("secret-token")}, &calls
}

func TestListOpenPullsReadsEveryPage(t *testing.T) {
	var page2 []map[string]any
	for i := range 3 {
		page2 = append(page2, map[string]any{"number": 200 + i, "title": "later", "user": map[string]string{"login": "bob", "type": "User"}})
	}
	var page1 []map[string]any
	for i := range pullsPerPage {
		page1 = append(page1, map[string]any{"number": i + 1, "node_id": "PR_x", "title": "t", "user": map[string]string{"login": "dependabot[bot]", "type": "Bot"}, "labels": []map[string]string{{"name": "upkeep"}}, "head": map[string]any{"ref": "feat/x", "sha": "abc", "repo": map[string]string{"full_name": "o/r"}}, "base": map[string]string{"ref": "main"}})
	}
	c, calls := fakeAPI(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if strings.Contains(r.URL.RawQuery, "page=2") {
			_ = json.NewEncoder(w).Encode(page2)
			return
		}
		_ = json.NewEncoder(w).Encode(page1)
	})
	got, err := c.ListOpenPulls(t.Context(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != pullsPerPage+3 || got[0].User.Type != "Bot" || got[0].Labels[0].Name != "upkeep" || got[0].Head.Repo.FullName != "o/r" || got[0].NodeID != "PR_x" {
		t.Fatalf("got %d pulls, first %+v", len(got), got[0])
	}
	if len(*calls) != 2 || (*calls)[0].auth != "Bearer secret-token" || !strings.HasPrefix((*calls)[0].path, "/repos/o/r/pulls?state=open") {
		t.Fatalf("calls = %+v", *calls)
	}
}

func TestGetPullAndDefaultBranch(t *testing.T) {
	c, _ := fakeAPI(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		switch r.URL.Path {
		case "/repos/o/r/pulls/7":
			_, _ = w.Write([]byte(`{"number":7,"title":"x","head":{"ref":"b","sha":"123"}}`))
		case "/repos/o/r":
			_, _ = w.Write([]byte(`{"default_branch":"trunk"}`))
		default:
			http.NotFound(w, r)
		}
	})
	pr, err := c.GetPull(t.Context(), "o", "r", 7)
	if err != nil || pr.Number != 7 || pr.Head.SHA != "123" {
		t.Fatalf("pull %+v error %v", pr, err)
	}
	if branch, err := c.DefaultBranch(t.Context(), "o", "r"); err != nil || branch != "trunk" {
		t.Fatalf("branch %q error %v", branch, err)
	}
	if _, err := c.GetPull(t.Context(), "o", "r", 8); err == nil {
		t.Fatal("a missing pull request returned no error")
	}
}

func TestCommentPostsTheBody(t *testing.T) {
	c, calls := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) { w.WriteHeader(http.StatusCreated) })
	if err := c.Comment(t.Context(), "o", "r", 4, "hello **there**"); err != nil {
		t.Fatal(err)
	}
	got := (*calls)[0]
	if got.method != http.MethodPost || got.path != "/repos/o/r/issues/4/comments" || !strings.Contains(got.body, `"body":"hello **there**"`) {
		t.Fatalf("call = %+v", got)
	}
}

func TestMergeUsesAMergeCommitPinnedToTheHead(t *testing.T) {
	c, calls := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) {
		_, _ = w.Write([]byte(`{"sha":"merged123","merged":true}`))
	})
	sha, err := c.Merge(t.Context(), "o", "r", 9, "headsha", "Bump x")
	if err != nil || sha != "merged123" {
		t.Fatalf("sha %q error %v", sha, err)
	}
	got := (*calls)[0]
	if got.method != http.MethodPut || got.path != "/repos/o/r/pulls/9/merge" {
		t.Fatalf("call = %+v", got)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(got.body), &body); err != nil || body["merge_method"] != "merge" || body["sha"] != "headsha" || body["commit_title"] != "Bump x" {
		t.Fatalf("body = %v (%v)", body, err)
	}
}

func TestMergeRefusalsAreReported(t *testing.T) {
	c, _ := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"Required status check \"test\" is expected."}`))
	})
	_, err := c.Merge(t.Context(), "o", "r", 9, "s", "t")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusMethodNotAllowed || !strings.Contains(apiErr.Message, "Required status check") {
		t.Fatalf("error = %v", err)
	}
	unconfirmed, _ := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) { _, _ = w.Write([]byte(`{"merged":false}`)) })
	if _, err := unconfirmed.Merge(t.Context(), "o", "r", 9, "s", "t"); err == nil {
		t.Fatal("an unconfirmed merge returned no error")
	}
}

func TestEnableAutoMerge(t *testing.T) {
	answers := map[string]string{
		"ok":       `{"data":{"enablePullRequestAutoMerge":{"pullRequest":{"number":3}}}}`,
		"clean":    `{"errors":[{"message":"Pull request Pull request is in clean status"}]}`,
		"disabled": `{"errors":[{"message":"Auto merge is not allowed for this repository"}]}`,
	}
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			c, calls := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) { _, _ = w.Write([]byte(answer)) })
			err := c.EnableAutoMerge(context.Background(), "PR_node")
			switch name {
			case "ok":
				if err != nil {
					t.Fatal(err)
				}
			case "clean":
				if !errors.Is(err, ErrAutoMergeClean) {
					t.Fatalf("error = %v", err)
				}
			default:
				if err == nil || errors.Is(err, ErrAutoMergeClean) || !strings.Contains(err.Error(), "not allowed") {
					t.Fatalf("error = %v", err)
				}
			}
			got := (*calls)[0]
			if got.path != "/graphql" || !strings.Contains(got.body, "enablePullRequestAutoMerge") || !strings.Contains(got.body, "PR_node") || !strings.Contains(got.body, "MERGE") {
				t.Fatalf("call = %+v", got)
			}
		})
	}
}

func TestTheTokenNeverAppearsInErrors(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1", Token: config.Secret("secret-token")}
	err := c.Comment(t.Context(), "o", "r", 1, "x")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error = %v", err)
	}
}
