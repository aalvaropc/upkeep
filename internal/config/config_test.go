package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

var fakeKey = strings.Repeat("k", 40)

func TestDefaults(t *testing.T) {
	c, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		NebiusBaseURL:    DefaultBaseURL,
		Models:           Models{Ultra: DefaultUltra, Super: DefaultSuper, Nano: DefaultNano},
		Mode:             Replay,
		Addr:             DefaultAddr,
		DataDir:          DefaultDataDir,
		Sandbox:          Docker,
		DailyTokens:      DefaultDailyTokens,
		SearchDailyLimit: DefaultSearchLimit,
	}
	if fmt.Sprintf("%#v", c) != fmt.Sprintf("%#v", want) {
		t.Fatalf("Load(nil) =\n%#v\nwant\n%#v", c, want)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load([]string{
		"NEBIUS_API_KEY=" + fakeKey,
		"NEBIUS_BASE_URL=https://example.test/v1/",
		"UPKEEP_MODEL_ULTRA=u",
		"UPKEEP_MODEL_SUPER=s",
		"UPKEEP_MODEL_NANO=n",
		"TAVILY_API_KEY=tvly",
		"GITHUB_TOKEN=gh",
		"UPKEEP_MODE=live",
		"UPKEEP_PUBLIC=true",
		"UPKEEP_ADDR=0.0.0.0:9000",
		"UPKEEP_DATA_DIR=/var/lib/upkeep",
		"UPKEEP_SANDBOX=exec",
		"UPKEEP_SANDBOX_RUNTIME=runsc",
		"UPKEEP_DAILY_TOKENS=5000",
		"UPKEEP_SEARCH_DAILY_LIMIT=0",
		"UPKEEP_PRICE_ULTRA_INPUT=0.6",
		"UPKEEP_PRICE_ULTRA_OUTPUT=1.8",
		"UPKEEP_PRICE_NANO_INPUT=0.06",
		"UPKEEP_PRICE_NANO_OUTPUT=0.24",
		"UPKEEP_PRICE_TAVILY_SEARCH=0.008",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.SearchPrice != 0.008 {
		t.Errorf("search price = %v", c.SearchPrice)
	}
	if c.NebiusAPIKey.Reveal() != fakeKey || c.TavilyAPIKey.Reveal() != "tvly" || c.GitHubToken.Reveal() != "gh" {
		t.Error("secrets were not loaded")
	}
	if c.NebiusBaseURL != "https://example.test/v1" {
		t.Errorf("base URL = %q, want the trailing slash trimmed", c.NebiusBaseURL)
	}
	if c.Models != (Models{Ultra: "u", Super: "s", Nano: "n"}) {
		t.Errorf("models = %+v", c.Models)
	}
	if c.Mode != Live || !c.Public || c.Addr != "0.0.0.0:9000" || c.DataDir != "/var/lib/upkeep" || c.Sandbox != Exec {
		t.Errorf("config = %+v", c)
	}
	if c.SandboxRuntime != Runsc {
		t.Errorf("sandbox runtime = %q", c.SandboxRuntime)
	}
	if c.DailyTokens != 5000 || c.SearchDailyLimit != 0 {
		t.Errorf("limits = %d tokens, %d searches", c.DailyTokens, c.SearchDailyLimit)
	}
	if c.Prices.Ultra == nil || *c.Prices.Ultra != (Price{Input: 0.6, Output: 1.8}) {
		t.Errorf("ultra price = %+v", c.Prices.Ultra)
	}
	if c.Prices.Super != nil {
		t.Errorf("super price = %+v, want unset", c.Prices.Super)
	}
	if c.Prices.Nano == nil || *c.Prices.Nano != (Price{Input: 0.06, Output: 0.24}) {
		t.Errorf("nano price = %+v", c.Prices.Nano)
	}
}

func TestInvalid(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"UPKEEP_MODE=fast", "UPKEEP_MODE"},
		{"UPKEEP_MODE=live", "needs NEBIUS_API_KEY"},
		{"UPKEEP_MODE=record", "needs NEBIUS_API_KEY"},
		{"UPKEEP_SANDBOX=vm", "UPKEEP_SANDBOX"},
		{"UPKEEP_SANDBOX_RUNTIME=kata", "UPKEEP_SANDBOX_RUNTIME"},
		{"UPKEEP_SANDBOX_RUNTIME=RUNSC", "UPKEEP_SANDBOX_RUNTIME"},
		{"UPKEEP_ADDR=8080", "UPKEEP_ADDR"},
		{"UPKEEP_PUBLIC=maybe", "UPKEEP_PUBLIC"},
		{"UPKEEP_DAILY_TOKENS=0", "UPKEEP_DAILY_TOKENS"},
		{"UPKEEP_DAILY_TOKENS=many", "UPKEEP_DAILY_TOKENS"},
		{"UPKEEP_SEARCH_DAILY_LIMIT=-1", "UPKEEP_SEARCH_DAILY_LIMIT"},
		{"UPKEEP_PRICE_TAVILY_SEARCH=-0.01", "UPKEEP_PRICE_TAVILY_SEARCH"},
		{"UPKEEP_PRICE_TAVILY_SEARCH=cheap", "UPKEEP_PRICE_TAVILY_SEARCH"},
		{"NEBIUS_BASE_URL=ftp://example.test", "NEBIUS_BASE_URL"},
		{"NEBIUS_BASE_URL=not a url", "NEBIUS_BASE_URL"},
		{"UPKEEP_PRICE_SUPER_INPUT=1", "must be set together"},
		{"UPKEEP_PRICE_SUPER_OUTPUT=1", "must be set together"},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			_, err := Load([]string{tt.env})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load(%q) error = %v, want it to mention %q", tt.env, err, tt.want)
			}
		})
	}
}

func TestTheSandboxRuntimeAcceptsTheDockerDefaultRuncAndRunsc(t *testing.T) {
	for _, v := range []string{"", Runc, Runsc} {
		c, err := Load([]string{"UPKEEP_SANDBOX_RUNTIME=" + v})
		if err != nil || c.SandboxRuntime != v {
			t.Errorf("UPKEEP_SANDBOX_RUNTIME=%q: runtime %q, error %v", v, c.SandboxRuntime, err)
		}
	}
}

func TestInvalidPrice(t *testing.T) {
	_, err := Load([]string{"UPKEEP_PRICE_NANO_INPUT=-1", "UPKEEP_PRICE_NANO_OUTPUT=x"})
	if err == nil || !strings.Contains(err.Error(), "UPKEEP_PRICE_NANO_INPUT") {
		t.Fatalf("error = %v", err)
	}
}

func TestSecretsNeverPrinted(t *testing.T) {
	c, err := Load([]string{"NEBIUS_API_KEY=" + fakeKey, "TAVILY_API_KEY=" + fakeKey, "GITHUB_TOKEN=" + fakeKey, "UPKEEP_MODE=live"})
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{
		"%v":  fmt.Sprintf("%v", c),
		"%+v": fmt.Sprintf("%+v", c),
		"%#v": fmt.Sprintf("%#v", c),
		"%s":  fmt.Sprintf("key=%s", c.NebiusAPIKey),
	}
	js, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	outputs["json"] = string(js)
	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("config", "config", c, "key", c.NebiusAPIKey)
	outputs["slog"] = logs.String()
	for name, out := range outputs {
		if strings.Contains(out, fakeKey) {
			t.Errorf("%s output leaks the secret: %s", name, out)
		}
	}
	if !strings.Contains(outputs["%+v"], redacted) {
		t.Errorf("%%+v output does not show %s: %s", redacted, outputs["%+v"])
	}
}

func TestEmptyValuesMeanUnset(t *testing.T) {
	c, err := Load([]string{"UPKEEP_MODE=", "UPKEEP_ADDR=  ", "UPKEEP_PRICE_ULTRA_INPUT=", "UPKEEP_PRICE_ULTRA_OUTPUT="})
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != Replay || c.Addr != DefaultAddr || c.Prices.Ultra != nil {
		t.Fatalf("config = %+v", c)
	}
}
