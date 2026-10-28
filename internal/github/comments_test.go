package github

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestListCommentsReadsEveryPage(t *testing.T) {
	var page1 []map[string]any
	for i := range commentsPerPage {
		page1 = append(page1, map[string]any{"id": i + 1, "body": "first page", "user": map[string]string{"login": "alice", "type": "User"}})
	}
	page2 := []map[string]any{{"id": 9001, "body": "<!-- upkeep:receipt -->\nUpkeep receipt", "user": map[string]string{"login": "upkeep-bot[bot]", "type": "Bot"}}}
	c, calls := fakeAPI(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(page2)
			return
		}
		_ = json.NewEncoder(w).Encode(page1)
	})
	got, err := c.ListComments(t.Context(), "o", "r", 4)
	if err != nil {
		t.Fatal(err)
	}
	last := got[len(got)-1]
	if len(got) != commentsPerPage+1 || last.ID != 9001 || last.User.Type != "Bot" || !strings.HasPrefix(last.Body, "<!-- upkeep:receipt -->") {
		t.Fatalf("got %d comments, last %+v", len(got), last)
	}
	if len(*calls) != 2 || (*calls)[0].method != http.MethodGet || (*calls)[0].auth != "Bearer secret-token" || !strings.HasPrefix((*calls)[0].path, "/repos/o/r/issues/4/comments?per_page=100&page=1") {
		t.Fatalf("calls = %+v", *calls)
	}
}

func TestListCommentsReportsBadAnswers(t *testing.T) {
	broken, _ := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) { _, _ = w.Write([]byte(`{"oops"`)) })
	if _, err := broken.ListComments(t.Context(), "o", "r", 4); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("error = %v", err)
	}
	missing, _ := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	var apiErr *Error
	if _, err := missing.ListComments(t.Context(), "o", "r", 4); !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("error = %v", err)
	}
}

func TestEditCommentPatchesTheBody(t *testing.T) {
	c, calls := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) { _, _ = w.Write([]byte(`{"id":77}`)) })
	if err := c.EditComment(t.Context(), "o", "r", 77, "updated **body**"); err != nil {
		t.Fatal(err)
	}
	got := (*calls)[0]
	if got.method != http.MethodPatch || got.path != "/repos/o/r/issues/comments/77" || got.auth != "Bearer secret-token" || !strings.Contains(got.body, `"body":"updated **body**"`) {
		t.Fatalf("call = %+v", got)
	}
}

func TestEditCommentRefusalsAreReported(t *testing.T) {
	c, _ := fakeAPI(t, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	})
	err := c.EditComment(t.Context(), "o", "r", 77, "x")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error = %v", err)
	}
}
