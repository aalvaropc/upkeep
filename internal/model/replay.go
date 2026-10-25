package model

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/fixture"
)

const FixtureKind = "model"

type Exchange struct {
	Model      string    `json:"model"`
	Request    Request   `json:"request"`
	Response   Response  `json:"response"`
	RecordedAt time.Time `json:"recorded_at"`
}

var (
	durationPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m)\b`)
	tempPathPattern = regexp.MustCompile(`(?:/tmp|/var/folders|/private/var)/[^\s:"'` + "`" + `)]+`)
	cachedPattern   = regexp.MustCompile(`\(cached\)`)
)

func Normalize(text string) string {
	text = tempPathPattern.ReplaceAllString(text, "<tmp>")
	text = durationPattern.ReplaceAllString(text, "<duration>")
	text = cachedPattern.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

func normalized(req Request) Request {
	msgs := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		m.Content = Normalize(m.Content)
		m.Reasoning = ""
		msgs[i] = m
	}
	req.Messages = msgs
	return req
}

func RequestKey(req Request) (string, error) {
	return fixture.Key(normalized(req))
}

type Recorder struct {
	Inner    Model
	Fixtures fixture.Store
	Now      func() time.Time
}

func (r *Recorder) Complete(ctx context.Context, req Request) (Response, error) {
	if r.Inner == nil || r.Fixtures == nil {
		return Response{}, errors.New("a recorder needs a model to call and a place to save exchanges")
	}
	key, err := RequestKey(req)
	if err != nil {
		return Response{}, err
	}
	resp, err := r.Inner.Complete(ctx, req)
	if err != nil {
		return Response{}, err
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	exchange := Exchange{Model: req.Model, Request: normalized(req), Response: resp, RecordedAt: now().UTC()}
	if err := r.Fixtures.Save(FixtureKind, key, exchange); err != nil {
		return Response{}, fmt.Errorf("saving the recorded exchange: %w", err)
	}
	return resp, nil
}

type Replayer struct {
	Fixtures fixture.Store
	Pace     float64
	Sleep    func(ctx context.Context, d time.Duration) error
}

func (r *Replayer) Complete(ctx context.Context, req Request) (Response, error) {
	if r.Fixtures == nil {
		return Response{}, errors.New("a replayer needs recorded exchanges to read")
	}
	key, err := RequestKey(req)
	if err != nil {
		return Response{}, err
	}
	var exchange Exchange
	if err := r.Fixtures.Load(FixtureKind, key, &exchange); err != nil {
		if errors.Is(err, fixture.ErrMiss) {
			return Response{}, fmt.Errorf("no recorded answer from %s for this request: %w", req.Model, err)
		}
		return Response{}, err
	}
	if r.Pace > 0 {
		sleep := r.Sleep
		if sleep == nil {
			sleep = sleepContext
		}
		if err := sleep(ctx, time.Duration(float64(exchange.Response.Latency)*r.Pace)); err != nil {
			return Response{}, err
		}
	}
	return exchange.Response, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
