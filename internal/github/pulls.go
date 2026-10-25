package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const pullsPerPage = 100
const maxPullPages = 5

func (c *Client) send(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token.IsSet() {
		req.Header.Set("Authorization", "Bearer "+c.Token.Reveal())
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, redactToken(err, c)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, redactToken(err, c)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var shaped struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &shaped)
		return nil, &Error{Status: resp.StatusCode, Message: shaped.Message, RateLimited: resp.Header.Get("X-RateLimit-Remaining") == "0"}
	}
	return raw, nil
}

func (c *Client) ListOpenPulls(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	var out []PullRequest
	for page := 1; page <= maxPullPages; page++ {
		raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls?state=open&per_page=%d&page=%d", owner, repo, pullsPerPage, page))
		if err != nil {
			return nil, err
		}
		var batch []PullRequest
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("github sent a pull request list that is not valid JSON: %w", err)
		}
		out = append(out, batch...)
		if len(batch) < pullsPerPage {
			break
		}
	}
	return out, nil
}

func (c *Client) GetPull(ctx context.Context, owner, repo string, number int) (PullRequest, error) {
	raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number))
	if err != nil {
		return PullRequest{}, err
	}
	var pr PullRequest
	if err := json.Unmarshal(raw, &pr); err != nil {
		return PullRequest{}, fmt.Errorf("github sent a pull request that is not valid JSON: %w", err)
	}
	return pr, nil
}

func (c *Client) DefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	raw, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s", owner, repo))
	if err != nil {
		return "", err
	}
	var doc struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.DefaultBranch == "" {
		return "", errors.New("github did not say what the default branch is")
	}
	return doc.DefaultBranch, nil
}

func (c *Client) Comment(ctx context.Context, owner, repo string, number int, body string) error {
	_, err := c.send(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number), map[string]string{"body": body})
	return err
}

func (c *Client) Merge(ctx context.Context, owner, repo string, number int, sha, title string) (string, error) {
	raw, err := c.send(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, number), map[string]string{
		"merge_method": "merge",
		"sha":          sha,
		"commit_title": title,
	})
	if err != nil {
		return "", err
	}
	var doc struct {
		SHA    string `json:"sha"`
		Merged bool   `json:"merged"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || !doc.Merged {
		return "", errors.New("github did not confirm the merge")
	}
	return doc.SHA, nil
}

var ErrAutoMergeClean = errors.New("the pull request is ready, so auto-merge does not apply")

func (c *Client) EnableAutoMerge(ctx context.Context, nodeID string) error {
	query := `mutation($id: ID!) { enablePullRequestAutoMerge(input: {pullRequestId: $id, mergeMethod: MERGE}) { pullRequest { number } } }`
	raw, err := c.send(ctx, http.MethodPost, "/graphql", map[string]any{"query": query, "variables": map[string]string{"id": nodeID}})
	if err != nil {
		return err
	}
	var doc struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("github sent an auto-merge answer that is not valid JSON: %w", err)
	}
	if len(doc.Errors) > 0 {
		msg := doc.Errors[0].Message
		if strings.Contains(strings.ToLower(msg), "clean status") {
			return ErrAutoMergeClean
		}
		return errors.New(msg)
	}
	return nil
}
