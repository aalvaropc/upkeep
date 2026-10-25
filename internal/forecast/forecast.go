package forecast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/github"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
)

const (
	CacheKind          = "forecast"
	maxReleases        = 8
	maxReleaseChars    = 1500
	maxSourceChars     = 12000
	maxExtraChars      = 3000
	maxSearchResults   = 4
	maxFilesListed     = 12
	forecastMaxTokens  = 1800
	systemInstructions = `You read release notes and migration guides for a Go module upgrade and tell a developer what will break. Answer with a single JSON object and nothing else:
{"summary":"<two sentences>","breaking":[{"symbol":"<a Go identifier exactly as it is written in the text, such as jwt.StandardClaims>","change":"<what replaced it or how it changed>"}]}
If a migration guide is present, base the answer on it and use the release notes only to complete it. Quote identifiers exactly as they appear in the text. Never invent identifiers, packages or signatures. If the project's identifiers are listed, report the changes that affect them first, and add others only when the text states them explicitly. If the text does not say, leave the list empty.`
	maxExcerptChars = 7000
)

type Release struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

type Result struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Content    string  `json:"content"`
	RawContent string  `json:"raw_content,omitempty"`
	Score      float64 `json:"score"`
}

type Releases interface {
	Between(ctx context.Context, module, from, to string) ([]Release, error)
}

type Search interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
}

type Change struct {
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
}

type Forecast struct {
	Module        string    `json:"module"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	Summary       string    `json:"summary"`
	Breaking      []Change  `json:"breaking"`
	Dropped       int       `json:"dropped"`
	Excerpt       string    `json:"excerpt"`
	ExcerptSource string    `json:"excerpt_source"`
	Files         []string  `json:"files"`
	Sources       []string  `json:"sources"`
	FetchedAt     time.Time `json:"fetched_at"`
}

type Cost struct {
	Model   string        `json:"model"`
	Usage   model.Usage   `json:"usage"`
	Charge  model.Charge  `json:"charge"`
	Latency time.Duration `json:"latency"`
	Cached  bool          `json:"cached"`
}

type Input struct {
	Module string
	From   string
	To     string
	Dir    string
}

type Forecaster struct {
	Releases  Releases
	Search    Search
	Model     model.Model
	ModelName string
	Meter     model.Meter
	Cache     fixture.Store
	Now       func() time.Time
}

func (f Forecast) Expects(symbol string) bool {
	return slices.ContainsFunc(f.Breaking, func(c Change) bool { return c.Symbol == symbol })
}

func identifier(symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if i := strings.LastIndex(symbol, "."); i >= 0 {
		symbol = symbol[i+1:]
	}
	return strings.Trim(symbol, "()*[]{}<>,;: \t")
}

func (f Forecast) Confirm(output string) (confirmed, missed []string) {
	for _, c := range f.Breaking {
		id := identifier(c.Symbol)
		if len(id) < 3 {
			continue
		}
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`)
		if pattern.MatchString(output) {
			confirmed = append(confirmed, c.Symbol)
		} else {
			missed = append(missed, c.Symbol)
		}
	}
	return confirmed, missed
}

func (f Forecast) Notes() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Upgrade of %s from %s to %s.", f.Module, f.From, f.To)
	if len(f.Breaking) > 0 {
		b.WriteString("\n\nBreaking changes found in the release notes and the migration guide:")
		for _, c := range f.Breaking {
			fmt.Fprintf(&b, "\n- %s: %s", c.Symbol, c.Description)
		}
	}
	if e := strings.TrimSpace(f.Excerpt); e != "" {
		source := "the migration guide"
		if f.ExcerptSource != "" {
			source = f.ExcerptSource
		}
		fmt.Fprintf(&b, "\n\nExcerpt from %s:\n\n%s", source, e)
	}
	return b.String()
}

func Query(module, from, to string) string {
	a, errA := pull.ParseVersion(from)
	b, errB := pull.ParseVersion(to)
	if errA == nil && errB == nil && a.Major != b.Major {
		return module + " migration guide"
	}
	return module + " " + to + " release notes changelog"
}

func Domains(module string) []string {
	if _, _, ok := github.ModuleRepo(module); ok {
		return []string{"github.com", "pkg.go.dev"}
	}
	return nil
}

type GitHubReleases struct {
	Client *github.Client
}

func (g GitHubReleases) Between(ctx context.Context, module, from, to string) ([]Release, error) {
	owner, repo, ok := github.ModuleRepo(module)
	if !ok {
		return nil, fmt.Errorf("%s is not hosted on GitHub", module)
	}
	low, err := pull.ParseVersion(from)
	if err != nil {
		return nil, err
	}
	high, err := pull.ParseVersion(to)
	if err != nil {
		return nil, err
	}
	all, err := g.Client.ListReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	type versioned struct {
		v pull.Version
		r Release
	}
	var picked []versioned
	for _, r := range all {
		tag := r.TagName
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		v, err := pull.ParseVersion(tag)
		if err != nil || (v.Prerelease() && !high.Prerelease()) {
			continue
		}
		if pull.CompareVersions(v, low) <= 0 || pull.CompareVersions(v, high) > 0 {
			continue
		}
		picked = append(picked, versioned{v: v, r: Release{Version: tag, Name: r.Name, Notes: r.Body, URL: r.HTMLURL, PublishedAt: r.PublishedAt}})
	}
	slices.SortFunc(picked, func(a, b versioned) int { return pull.CompareVersions(b.v, a.v) })
	out := make([]Release, 0, min(len(picked), maxReleases))
	for _, p := range picked[:min(len(picked), maxReleases)] {
		out = append(out, p.r)
	}
	return out, nil
}

func (f Forecaster) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func cacheKey(in Input) (string, error) {
	return fixture.Key(struct{ Module, From, To string }{in.Module, in.From, in.To})
}

func (f Forecaster) Forecast(ctx context.Context, in Input) (Forecast, Cost, error) {
	if in.Module == "" || in.From == "" || in.To == "" {
		return Forecast{}, Cost{}, errors.New("a forecast needs a module and both versions")
	}
	key, err := cacheKey(in)
	if err != nil {
		return Forecast{}, Cost{}, err
	}
	if f.Cache != nil {
		var cached Forecast
		switch err := f.Cache.Load(CacheKind, key, &cached); {
		case err == nil:
			cached.Files = importers(in.Dir, in.Module)
			return cached, Cost{Cached: true}, nil
		case !errors.Is(err, fixture.ErrMiss):
			return Forecast{}, Cost{}, err
		}
	}
	if f.Model == nil {
		return Forecast{}, Cost{}, errors.New("no recorded forecast and no model to write one")
	}

	sources, urls, excerpt, excerptSource := f.gather(ctx, in)
	if sources == "" {
		return Forecast{}, Cost{}, errors.New("no release notes or migration guide could be found")
	}
	files := importers(in.Dir, in.Module)
	user := fmt.Sprintf("Module: %s\nUpgrading from %s to %s.\n", in.Module, in.From, in.To)
	if len(files) > 0 {
		user += "The project imports it in: " + strings.Join(files, ", ") + ".\n"
	}
	if symbols := uses(in.Dir, in.Module); len(symbols) > 0 {
		user += "The project uses these identifiers from it, written as the project writes them: " + strings.Join(symbols, ", ") + ".\n"
	}
	user += "\n" + sources

	off := false
	resp, err := f.Model.Complete(ctx, model.Request{
		Model:     f.ModelName,
		MaxTokens: forecastMaxTokens,
		Reasoning: &off,
		Messages:  []model.Message{{Role: "system", Content: systemInstructions}, {Role: "user", Content: user}},
	})
	if err != nil {
		return Forecast{}, Cost{}, fmt.Errorf("writing the forecast: %w", err)
	}
	cost := Cost{Model: f.ModelName, Usage: resp.Usage, Charge: f.Meter.Charge(f.ModelName, resp.Usage), Latency: resp.Latency}

	out, err := parse(resp.Message.Content)
	if err != nil {
		return Forecast{}, cost, err
	}
	out.Breaking, out.Dropped = ground(out.Breaking, sources)
	out.Excerpt, out.ExcerptSource = excerpt, excerptSource
	out.Module, out.From, out.To = in.Module, in.From, in.To
	out.Files = files
	out.Sources = urls
	out.FetchedAt = f.now().UTC()
	if f.Cache != nil {
		stored := out
		stored.Files = nil
		if err := f.Cache.Save(CacheKind, key, stored); err != nil {
			return out, cost, fmt.Errorf("saving the forecast: %w", err)
		}
	}
	return out, cost, nil
}

func (f Forecaster) gather(ctx context.Context, in Input) (text string, urls []string, excerpt, excerptSource string) {
	var b strings.Builder
	if f.Search != nil {
		if results, err := f.Search.Search(ctx, Query(in.Module, in.From, in.To), maxSearchResults); err == nil {
			for i, r := range results {
				text, limit := CleanPage(r.RawContent), maxSourceChars
				if text == "" || i > 0 {
					text, limit = strings.TrimSpace(r.Content), maxExtraChars
				}
				if text == "" {
					continue
				}
				fmt.Fprintf(&b, "\nSearch result: %s (%s)\n%s\n", r.Title, r.URL, truncate(text, limit))
				urls = append(urls, r.URL)
				if excerpt == "" && i == 0 {
					excerpt, excerptSource = truncate(text, maxExcerptChars), r.URL
				}
			}
		}
	}
	if f.Releases != nil {
		if releases, err := f.Releases.Between(ctx, in.Module, in.From, in.To); err == nil && len(releases) > 0 {
			b.WriteString("\nRelease notes, newest first:\n")
			for _, r := range releases {
				fmt.Fprintf(&b, "\n## %s\n%s\n", r.Version, truncate(strings.TrimSpace(r.Notes), maxReleaseChars))
				if r.URL != "" {
					urls = append(urls, r.URL)
				}
			}
		}
	}
	return b.String(), dedupe(urls), excerpt, excerptSource
}

var (
	markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	pageDebris   = regexp.MustCompile(`^(/|\d+ lines.*·.*)$`)
	pageChrome   = []string{
		"skip to content", "sign in", "sign up", "appearance settings", "notifications", "fork ", "star ", "{{ message }}",
		"you signed in", "you signed out", "you switched accounts", "reload to refresh", "dismiss alert", "copy path",
		"more file actions", "expand file tree", "latest commit", "history", "file metadata and controls", "navigation menu",
		"copy raw file", "download raw file", "outline", "edit and raw actions",
	}
)

func lastNonBlank(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			return lines[i]
		}
	}
	return ""
}

func CleanPage(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		lower := strings.ToLower(strings.Trim(markdownLink.ReplaceAllString(s, "$1"), "*#- "))
		if slices.ContainsFunc(pageChrome, func(prefix string) bool { return strings.HasPrefix(lower, prefix) }) {
			continue
		}
		if pageDebris.MatchString(s) {
			continue
		}
		heading := strings.HasPrefix(s, "#")
		if !heading && len(s) > 0 && float64(len(s)-len(markdownLink.ReplaceAllString(s, ""))) > 0.6*float64(len(s)) {
			continue
		}
		if heading {
			s = markdownLink.ReplaceAllString(s, "$1")
		}
		if heading && lastNonBlank(out) == s {
			continue
		}
		out = append(out, s)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... (cut)"
}

func parse(answer string) (Forecast, error) {
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if start < 0 || end <= start {
		return Forecast{}, errors.New("the forecast is not a JSON object")
	}
	var doc struct {
		Summary  string `json:"summary"`
		Breaking []struct {
			Symbol string `json:"symbol"`
			Change string `json:"change"`
		} `json:"breaking"`
	}
	if err := json.Unmarshal([]byte(answer[start:end+1]), &doc); err != nil {
		return Forecast{}, fmt.Errorf("the forecast is not valid JSON: %w", err)
	}
	out := Forecast{Summary: strings.TrimSpace(doc.Summary)}
	for _, c := range doc.Breaking {
		if strings.TrimSpace(c.Symbol) == "" {
			continue
		}
		out.Breaking = append(out.Breaking, Change{Symbol: strings.TrimSpace(c.Symbol), Description: strings.TrimSpace(c.Change)})
	}
	if out.Summary == "" && len(out.Breaking) == 0 {
		return Forecast{}, errors.New("the forecast is empty")
	}
	return out, nil
}

func ground(changes []Change, sources string) (kept []Change, dropped int) {
	for _, c := range changes {
		id := identifier(c.Symbol)
		if len(id) >= 3 && strings.Contains(sources, id) {
			kept = append(kept, c)
		} else {
			dropped++
		}
	}
	return kept, dropped
}

func importers(dir, module string) []string {
	if dir == "" {
		return nil
	}
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || len(files) >= maxFilesListed {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, imp := range file.Imports {
			if p, err := strconv.Unquote(imp.Path.Value); err == nil && (p == module || strings.HasPrefix(p, module+"/")) {
				if rel, err := filepath.Rel(dir, path); err == nil {
					files = append(files, filepath.ToSlash(rel))
				}
				break
			}
		}
		return nil
	})
	slices.Sort(files)
	return files
}
