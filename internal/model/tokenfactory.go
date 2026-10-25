package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
)

const (
	DefaultMaxRetries = 3
	maxErrorBody      = 2048
	maxResponseBody   = 8 << 20
)

type APIError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("token factory returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("token factory returned HTTP %d: %s", e.Status, e.Message)
}

func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

type TokenFactory struct {
	BaseURL    string
	Key        config.Secret
	HTTP       *http.Client
	MaxRetries int
	Backoff    func(attempt int, retryAfter time.Duration) time.Duration
}

func NewTokenFactory(baseURL string, key config.Secret) *TokenFactory {
	return &TokenFactory{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Key:        key,
		HTTP:       &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 150 * time.Second, MaxIdleConnsPerHost: 8, IdleConnTimeout: 60 * time.Second}},
		MaxRetries: DefaultMaxRetries,
	}
}

func DefaultBackoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, 30*time.Second)
	}
	base := time.Duration(1<<attempt) * time.Second
	jitter := time.Duration(rand.Int64N(int64(base) / 4))
	return min(base+jitter, 30*time.Second)
}

func (t *TokenFactory) Complete(ctx context.Context, req Request) (Response, error) {
	if req.Model == "" {
		return Response{}, errors.New("the request has no model")
	}
	body, err := json.Marshal(toWire(req))
	if err != nil {
		return Response{}, err
	}
	retries := max(t.MaxRetries, 0)
	backoff := t.Backoff
	if backoff == nil {
		backoff = DefaultBackoff
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		start := time.Now()
		resp, err := t.once(ctx, body)
		if err == nil {
			resp.Latency = time.Since(start)
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == retries || !retryable(err) {
			break
		}
		var retryAfter time.Duration
		if apiErr, ok := errors.AsType[*APIError](err); ok {
			retryAfter = apiErr.RetryAfter
		}
		timer := time.NewTimer(backoff(attempt, retryAfter))
		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{}, errors.Join(lastErr, ctx.Err())
		case <-timer.C:
		}
	}
	return Response{}, lastErr
}

func retryable(err error) bool {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.Retryable()
	}
	_, ok := errors.AsType[*transportError](err)
	return ok
}

type transportError struct{ err error }

func (e *transportError) Error() string { return "token factory request failed: " + e.err.Error() }

func (e *transportError) Unwrap() error { return e.err }

func (t *TokenFactory) once(ctx context.Context, body []byte) (Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+t.Key.Reveal())
	client := t.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return Response{}, &transportError{err: sanitize(err, t.Key)}
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(httpResp.Body, maxErrorBody))
		return Response{}, &APIError{
			Status:     httpResp.StatusCode,
			Message:    errorMessage(raw),
			RetryAfter: parseRetryAfter(httpResp.Header.Get("Retry-After")),
		}
	}
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBody))
	if err != nil {
		return Response{}, &transportError{err: sanitize(err, t.Key)}
	}
	var wire wireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Response{}, fmt.Errorf("token factory sent a response that is not valid JSON: %w", err)
	}
	return fromWire(wire)
}

func sanitize(err error, key config.Secret) error {
	if !key.IsSet() {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, key.Reveal()) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, key.Reveal(), "[redacted]"))
}

func errorMessage(raw []byte) string {
	var shaped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Detail  string `json:"detail"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &shaped) == nil {
		switch {
		case shaped.Error.Message != "":
			return shaped.Error.Message
		case shaped.Detail != "":
			return shaped.Detail
		case shaped.Message != "":
			return shaped.Message
		}
	}
	return strings.TrimSpace(string(raw))
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs >= 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if at, err := http.ParseTime(v); err == nil {
		return max(0, time.Until(at))
	}
	return 0
}
