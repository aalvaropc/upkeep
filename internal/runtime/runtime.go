package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/engine"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/forecast"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/gitx"
	"github.com/aalvaropc/upkeep/internal/learn"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/policy"
	"github.com/aalvaropc/upkeep/internal/queue"
	"github.com/aalvaropc/upkeep/internal/sandbox"
	"github.com/aalvaropc/upkeep/internal/strategy"
	"github.com/aalvaropc/upkeep/internal/tavily"
)

const (
	metaKind     = "meta"
	pricesID     = "prices"
	probeTimeout = 20 * time.Second
)

type Options struct {
	Config   config.Config
	Policy   policy.Policy
	Repo     string
	Git      gitx.Git
	Fixtures fixture.Store
	Record   fixture.Store
	CacheDir string
	Pace     float64
	Logger   *slog.Logger
}

type set struct {
	mode       config.Mode
	router     *model.Router
	meter      model.Meter
	forecaster strategy.Forecaster
}

type Runtime struct {
	opts     Options
	executor sandbox.Executor
	runner   sandbox.Runner
	primary  *set
	replay   *set
	probe    func(context.Context) error
	Budget   *Budget
}

type metered struct {
	inner  model.Model
	budget *Budget
}

func (m metered) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	resp, err := m.inner.Complete(ctx, req)
	if err == nil {
		m.budget.Add(resp.Usage.Total())
	}
	return resp, err
}

func New(ctx context.Context, o Options) (*Runtime, error) {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	r := &Runtime{opts: o}
	switch o.Config.Sandbox {
	case config.Docker:
		d := sandbox.Docker{CacheDir: filepath.Join(o.CacheDir, "go")}
		if err := d.EnsureImage(ctx); err != nil {
			return nil, fmt.Errorf("preparing the sandbox image: %w", err)
		}
		r.executor = d
	default:
		r.executor = sandbox.Exec{}
	}
	r.runner = sandbox.GoRunner{Executor: r.executor, Cache: fixture.Dir{Path: filepath.Join(o.CacheDir, "reports")}}

	r.Budget = &Budget{Path: filepath.Join(o.CacheDir, "usage.json"), Limit: o.Config.DailyTokens}
	fixtures := o.Fixtures
	if fixtures == nil {
		fixtures = fixture.None{}
	}

	var err error
	switch o.Config.Mode {
	case config.Live:
		if r.primary, err = r.networked(ctx, config.Live, fixture.Dir{Path: filepath.Join(o.CacheDir, "forecasts")}, nil); err != nil {
			return nil, err
		}
		if o.Fixtures != nil {
			if r.replay, err = r.replayed(fixtures); err != nil {
				o.Logger.Warn("recorded runs are not usable as a fallback", "error", err)
			}
		}
	case config.Record:
		if o.Record == nil {
			return nil, errors.New("record mode needs somewhere to save the exchanges")
		}
		if r.primary, err = r.networked(ctx, config.Record, o.Record, o.Record); err != nil {
			return nil, err
		}
	case config.Replay:
		if r.primary, err = r.replayed(fixtures); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown mode %q", o.Config.Mode)
	}
	return r, nil
}

func (r *Runtime) prices(book model.PriceBook) model.PriceBook {
	return model.ApplyOverrides(book, r.opts.Config.Models, r.opts.Config.Prices)
}

func (r *Runtime) networked(ctx context.Context, mode config.Mode, cache fixture.Store, record fixture.Store) (*set, error) {
	cfg := r.opts.Config
	if !cfg.NebiusAPIKey.IsSet() {
		return nil, errors.New("NEBIUS_API_KEY is required to call Token Factory")
	}
	tf := model.NewTokenFactory(cfg.NebiusBaseURL, cfg.NebiusAPIKey)
	book, err := tf.FetchPrices(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching prices: %w", err)
	}
	book = r.prices(book)
	if record != nil {
		if err := record.Save(metaKind, pricesID, book); err != nil {
			return nil, fmt.Errorf("saving prices: %w", err)
		}
	}
	var base model.Model = tf
	if record != nil {
		base = &model.Recorder{Inner: tf, Fixtures: record}
	}
	if mode == config.Live {
		base = metered{inner: base, budget: r.Budget}
		off := false
		r.probe = func(ctx context.Context) error {
			_, err := tf.Complete(ctx, model.Request{Model: cfg.Models.Nano, Messages: []model.Message{{Role: "user", Content: "Reply with OK."}}, MaxTokens: 1, Reasoning: &off})
			return err
		}
	}
	router := model.NewRouter()
	router.Set(model.Ultra, cfg.Models.Ultra, base)
	router.Set(model.Super, cfg.Models.Super, base)
	router.Set(model.Nano, cfg.Models.Nano, base)
	meter := model.Meter{Prices: book, Baseline: cfg.Models.Ultra}
	f := forecast.Forecaster{
		Releases:  forecast.GitHubReleases{Client: &github.Client{Token: cfg.GitHubToken}},
		Model:     base,
		ModelName: cfg.Models.Super,
		Role:      model.Super,
		Meter:     meter,
		Cache:     cache,
	}
	if cfg.TavilyAPIKey.IsSet() {
		f.Search = tavily.New(cfg.TavilyAPIKey, "github.com", "pkg.go.dev")
	}
	return &set{mode: mode, router: router, meter: meter, forecaster: f}, nil
}

func (r *Runtime) replayed(fixtures fixture.Store) (*set, error) {
	cfg := r.opts.Config
	var book model.PriceBook
	if err := fixtures.Load(metaKind, pricesID, &book); err != nil && !errors.Is(err, fixture.ErrMiss) {
		return nil, fmt.Errorf("reading recorded prices: %w", err)
	}
	book = r.prices(book)
	player := &model.Replayer{Fixtures: fixtures, Pace: r.opts.Pace}
	router := model.NewRouter()
	router.Set(model.Ultra, cfg.Models.Ultra, player)
	router.Set(model.Super, cfg.Models.Super, player)
	router.Set(model.Nano, cfg.Models.Nano, player)
	meter := model.Meter{Prices: book, Baseline: cfg.Models.Ultra}
	return &set{mode: config.Replay, router: router, meter: meter, forecaster: forecast.Forecaster{Cache: fixtures, Meter: meter, Replay: true, Pace: r.opts.Pace}}, nil
}

func (r *Runtime) Mode() config.Mode { return r.opts.Config.Mode }

func (r *Runtime) Processor(emit queue.Emitter, learned learn.Book) (queue.Processor, error) {
	chosen := r.primary
	if reason := r.liveUnavailable(); reason != "" {
		if r.replay == nil {
			return nil, errors.New(reason + " and there are no recorded runs to replay")
		}
		chosen = r.replay
		if emit != nil {
			emit(events.Notice, 0, map[string]string{"message": strings.ToUpper(reason[:1]) + reason[1:] + ", so this run replays a recorded one."})
		}
	}
	return engine.Processor{Env: strategy.Env{
		Forecaster: chosen.forecaster,
		Git:        r.opts.Git,
		Runner:     r.runner,
		Executor:   r.executor,
		Models:     chosen.router,
		Meter:      chosen.meter,
		Policy:     r.opts.Policy,
		Emit:       emit,
	}, Learned: learned, Repo: r.opts.Repo}, nil
}

func (r *Runtime) liveUnavailable() string {
	if r.opts.Config.Mode != config.Live {
		return ""
	}
	if r.Budget.Exhausted() {
		return "the live token budget for today is used up"
	}
	if r.probe == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	if err := r.probe(ctx); err != nil {
		r.opts.Logger.Warn("token factory failed the check before a live run", "error", err)
		return "a check call to Token Factory failed"
	}
	return ""
}
