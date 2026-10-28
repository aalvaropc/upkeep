package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	commentsPerPage = 100
	maxCommentPages = 10
)

type IssueComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User User   `json:"user"`
}

func (c *Client) ListComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	var out []IssueComment
	for page := 1; page <= maxCommentPages; page++ {
		raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=%d&page=%d", owner, repo, number, commentsPerPage, page))
		if err != nil {
			return nil, err
		}
		var batch []IssueComment
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("github sent a comment list that is not valid JSON: %w", err)
		}
		out = append(out, batch...)
		if len(batch) < commentsPerPage {
			break
		}
	}
	return out, nil
}

func (c *Client) EditComment(ctx context.Context, owner, repo string, id int64, body string) error {
	_, err := c.send(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/issues/comments/%d", owner, repo, id), map[string]string{"body": body})
	return err
}
