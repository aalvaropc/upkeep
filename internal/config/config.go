package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Secret string

const redacted = "[redacted]"

func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

func (s Secret) GoString() string { return strconv.Quote(s.String()) }

func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

func (s Secret) Reveal() string { return string(s) }

func (s Secret) IsSet() bool { return s != "" }

type Mode string

const (
	Live   Mode = "live"
	Replay Mode = "replay"
	Record Mode = "record"
)

type Sandbox string

const (
	Docker Sandbox = "docker"
	Exec   Sandbox = "exec"
)

type Models struct {
	Ultra string
	Super string
	Nano  string
}

type Price struct {
	Input  float64
	Output float64
}

type Prices struct {
	Ultra *Price
	Super *Price
	Nano  *Price
}

type Config struct {
	NebiusAPIKey     Secret
	NebiusBaseURL    string
	Models           Models
	TavilyAPIKey     Secret
	GitHubToken      Secret
	Mode             Mode
	Public           bool
	Addr             string
	DataDir          string
	Sandbox          Sandbox
	DailyTokens      int64
	SearchDailyLimit int
	Prices           Prices
}

const (
	DefaultBaseURL     = "https://api.tokenfactory.nebius.com/v1"
	DefaultUltra       = "nvidia/Nemotron-3-Ultra-550b-a55b"
	DefaultSuper       = "nvidia/nemotron-3-super-120b-a12b"
	DefaultNano        = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B"
	DefaultAddr        = "127.0.0.1:8080"
	DefaultDataDir     = "data"
	DefaultDailyTokens = 1_000_000
	DefaultSearchLimit = 100
)

func Load(environ []string) (Config, error) {
	env := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			env[k] = strings.TrimSpace(v)
		}
	}
	get := func(key, fallback string) string {
		if v := env[key]; v != "" {
			return v
		}
		return fallback
	}

	var errs []error
	c := Config{
		NebiusAPIKey:  Secret(env["NEBIUS_API_KEY"]),
		NebiusBaseURL: strings.TrimRight(get("NEBIUS_BASE_URL", DefaultBaseURL), "/"),
		Models: Models{
			Ultra: get("UPKEEP_MODEL_ULTRA", DefaultUltra),
			Super: get("UPKEEP_MODEL_SUPER", DefaultSuper),
			Nano:  get("UPKEEP_MODEL_NANO", DefaultNano),
		},
		TavilyAPIKey: Secret(env["TAVILY_API_KEY"]),
		GitHubToken:  Secret(env["GITHUB_TOKEN"]),
		Mode:         Mode(get("UPKEEP_MODE", string(Replay))),
		Addr:         get("UPKEEP_ADDR", DefaultAddr),
		DataDir:      get("UPKEEP_DATA_DIR", DefaultDataDir),
		Sandbox:      Sandbox(get("UPKEEP_SANDBOX", string(Docker))),
	}

	if u, err := url.Parse(c.NebiusBaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		errs = append(errs, fmt.Errorf("NEBIUS_BASE_URL %q is not an http or https URL", c.NebiusBaseURL))
	}
	switch c.Mode {
	case Live, Replay, Record:
	default:
		errs = append(errs, fmt.Errorf("UPKEEP_MODE %q must be live, replay or record", c.Mode))
	}
	if (c.Mode == Live || c.Mode == Record) && !c.NebiusAPIKey.IsSet() {
		errs = append(errs, fmt.Errorf("UPKEEP_MODE %s needs NEBIUS_API_KEY", c.Mode))
	}
	switch c.Sandbox {
	case Docker, Exec:
	default:
		errs = append(errs, fmt.Errorf("UPKEEP_SANDBOX %q must be docker or exec", c.Sandbox))
	}
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		errs = append(errs, fmt.Errorf("UPKEEP_ADDR %q: %w", c.Addr, err))
	}

	var err error
	if c.Public, err = strconv.ParseBool(get("UPKEEP_PUBLIC", "false")); err != nil {
		errs = append(errs, fmt.Errorf("UPKEEP_PUBLIC %q is not a boolean", env["UPKEEP_PUBLIC"]))
	}
	if c.DailyTokens, err = strconv.ParseInt(get("UPKEEP_DAILY_TOKENS", strconv.Itoa(DefaultDailyTokens)), 10, 64); err != nil || c.DailyTokens <= 0 {
		errs = append(errs, fmt.Errorf("UPKEEP_DAILY_TOKENS %q must be a positive integer", env["UPKEEP_DAILY_TOKENS"]))
	}
	if c.SearchDailyLimit, err = strconv.Atoi(get("UPKEEP_SEARCH_DAILY_LIMIT", strconv.Itoa(DefaultSearchLimit))); err != nil || c.SearchDailyLimit < 0 {
		errs = append(errs, fmt.Errorf("UPKEEP_SEARCH_DAILY_LIMIT %q must be a non-negative integer", env["UPKEEP_SEARCH_DAILY_LIMIT"]))
	}

	for _, p := range []struct {
		name string
		dst  **Price
	}{{"ULTRA", &c.Prices.Ultra}, {"SUPER", &c.Prices.Super}, {"NANO", &c.Prices.Nano}} {
		price, err := loadPrice(env, p.name)
		if err != nil {
			errs = append(errs, err)
		}
		*p.dst = price
	}

	return c, errors.Join(errs...)
}

func loadPrice(env map[string]string, name string) (*Price, error) {
	inKey, outKey := "UPKEEP_PRICE_"+name+"_INPUT", "UPKEEP_PRICE_"+name+"_OUTPUT"
	in, out := env[inKey], env[outKey]
	if in == "" && out == "" {
		return nil, nil
	}
	if in == "" || out == "" {
		return nil, fmt.Errorf("%s and %s must be set together", inKey, outKey)
	}
	var p Price
	var err error
	if p.Input, err = strconv.ParseFloat(in, 64); err != nil || p.Input < 0 {
		return nil, fmt.Errorf("%s %q must be a non-negative number", inKey, in)
	}
	if p.Output, err = strconv.ParseFloat(out, 64); err != nil || p.Output < 0 {
		return nil, fmt.Errorf("%s %q must be a non-negative number", outKey, out)
	}
	return &p, nil
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("mode", string(c.Mode)),
		slog.String("addr", c.Addr),
		slog.Bool("public", c.Public),
		slog.String("data_dir", c.DataDir),
		slog.String("sandbox", string(c.Sandbox)),
		slog.String("base_url", c.NebiusBaseURL),
		slog.String("ultra", c.Models.Ultra),
		slog.String("super", c.Models.Super),
		slog.String("nano", c.Models.Nano),
		slog.Bool("nebius_key", c.NebiusAPIKey.IsSet()),
		slog.Bool("tavily_key", c.TavilyAPIKey.IsSet()),
		slog.Bool("github_token", c.GitHubToken.IsSet()),
		slog.Int64("daily_tokens", c.DailyTokens),
		slog.Int("search_daily_limit", c.SearchDailyLimit),
	)
}
