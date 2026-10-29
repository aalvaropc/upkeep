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
	"github.com/aalvaropc/upkeep/internal/jsonfix"
	"github.com/aalvaropc/upkeep/internal/model"
	"github.com/aalvaropc/upkeep/internal/pull"
)

const (
	CacheKind          = "forecast"
	ReleasesKind       = "releases"
	ReleasesTTL        = 24 * time.Hour
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

type SearchHit struct {
	Title string  `json:"title"`
	URL   string  `json:"url"`
	Score float64 `json:"score"`
	Kept  bool    `json:"kept"`
}

type SearchTrace struct {
	Provider  string      `json:"provider"`
	Query     string      `json:"query"`
	Results   []SearchHit `json:"results,omitempty"`
	Kept      []string    `json:"kept,omitempty"`
	LatencyMS int64       `json:"latency_ms,omitempty"`
	Micros    int64       `json:"micros,omitempty"`
	Error     string      `json:"error,omitempty"`
	At        time.Time   `json:"at"`
	Recorded  bool        `json:"recorded,omitempty"`
	Reused    bool        `json:"reused,omitempty"`
	New       []string    `json:"new,omitempty"`
	Skipped   string      `json:"skipped,omitempty"`
}

type SkipError struct {
	Reason string
}

func (e *SkipError) Error() string { return e.Reason }

func ProviderOf(s Search) string {
	if named, ok := s.(interface{ Provider() string }); ok {
		return named.Provider()
	}
	return "search"
}

func ProviderLabel(provider string) string {
	if provider == "" {
		return "Search"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

type Forecast struct {
	Module        string       `json:"module"`
	From          string       `json:"from"`
	To            string       `json:"to"`
	Summary       string       `json:"summary"`
	Breaking      []Change     `json:"breaking"`
	Dropped       int          `json:"dropped"`
	Excerpt       string       `json:"excerpt"`
	ExcerptSource string       `json:"excerpt_source"`
	Files         []string     `json:"files"`
	Sources       []string     `json:"sources"`
	FetchedAt     time.Time    `json:"fetched_at"`
	Search        *SearchTrace `json:"search,omitempty"`
	Releases      []string     `json:"releases,omitempty"`
	ReleasesError string       `json:"releases_error,omitempty"`
}

type Cost struct {
	Role    model.Role    `json:"role,omitempty"`
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
	Releases     Releases
	Search       Search
	Model        model.Model
	ModelName    string
	Role         model.Role
	Meter        model.Meter
	Cache        fixture.Store
	Replay       bool
	Fresh        bool
	SearchMicros int64
	Pace         float64
	Now          func() time.Time
}

type recorded struct {
	Forecast Forecast `json:"forecast"`
	Cost     Cost     `json:"cost"`
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

func About(rawURL, module string) bool {
	owner, repo, ok := github.ModuleRepo(module)
	if !ok {
		return true
	}
	path := strings.ToLower(owner + "/" + repo)
	pattern := regexp.MustCompile(`(?:^|[/.])(?:github\.com|pkg\.go\.dev/github\.com)/` + regexp.QuoteMeta(path) + `(?:[/@?#]|$)`)
	return pattern.MatchString(strings.ToLower(rawURL))
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

func (f Forecaster) pause(ctx context.Context, d time.Duration) error {
	d = time.Duration(float64(d) * f.Pace)
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

func (f Forecaster) role() model.Role {
	if f.Role != "" {
		return f.Role
	}
	return model.Super
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
	var cached *Forecast
	if f.Cache != nil {
		var stored recorded
		switch err := f.Cache.Load(CacheKind, key, &stored); {
		case err == nil:
			if f.Replay {
				return f.replay(ctx, in, stored)
			}
			if !f.Fresh || f.Search == nil {
				return keep(in, stored.Forecast, stored.Forecast.Search), Cost{Cached: true}, nil
			}
			cached = &stored.Forecast
		case !errors.Is(err, fixture.ErrMiss):
			return Forecast{}, Cost{}, err
		}
	}
	if cached == nil && f.Model == nil {
		return Forecast{}, Cost{}, errors.New("no recorded forecast and no model to write one")
	}

	pages, trace := f.search(ctx, in)
	if cached != nil {
		if trace.Error == "" && trace.Skipped == "" {
			trace.Reused = true
			trace.New = unseen(trace.Kept, cached.Sources)
		}
		return keep(in, *cached, trace), Cost{Cached: true}, nil
	}
	out, cost, err := f.write(ctx, in, pages, trace)
	if err != nil {
		if cached == nil {
			return out, cost, err
		}
		if cost.Model == "" {
			cost.Cached = true
		}
		return keep(in, *cached, trace), cost, nil
	}
	if f.Cache != nil {
		stored := recorded{Forecast: out, Cost: cost}
		stored.Forecast.Files = nil
		if err := f.Cache.Save(CacheKind, key, stored); err != nil {
			return out, cost, fmt.Errorf("saving the forecast: %w", err)
		}
	}
	return out, cost, nil
}

func keep(in Input, fc Forecast, trace *SearchTrace) Forecast {
	fc.Files = importers(in.Dir, in.Module)
	fc.Search = trace
	return fc
}

func unseen(kept, sources []string) []string {
	var out []string
	for _, u := range kept {
		if !slices.Contains(sources, u) {
			out = append(out, u)
		}
	}
	return out
}

func (f Forecaster) replay(ctx context.Context, in Input, stored recorded) (Forecast, Cost, error) {
	out := stored.Forecast
	out.Files = importers(in.Dir, in.Module)
	if out.Search != nil {
		trace := *out.Search
		trace.Recorded = true
		out.Search = &trace
	}
	cost := stored.Cost
	cost.Cached = false
	if err := f.pause(ctx, cost.Latency); err != nil {
		return Forecast{}, Cost{}, err
	}
	return out, cost, nil
}

func (f Forecaster) write(ctx context.Context, in Input, pages []Result, trace *SearchTrace) (Forecast, Cost, error) {
	partial := Forecast{Module: in.Module, From: in.From, To: in.To, Search: trace}
	if f.Model == nil {
		return partial, Cost{}, errors.New("no model to write a new forecast")
	}
	releases, relErr := f.releases(ctx, in)
	if relErr != nil {
		partial.ReleasesError = relErr.Error()
	}
	partial.Releases = releaseURLs(releases)
	sources, urls, excerpt, excerptSource := assemble(pages, releases)
	if sources == "" {
		return partial, Cost{}, nothingFound(trace, partial.ReleasesError)
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
		return partial, Cost{}, fmt.Errorf("writing the forecast: %w", err)
	}
	cost := Cost{Role: f.role(), Model: f.ModelName, Usage: resp.Usage, Charge: f.Meter.Charge(f.ModelName, resp.Usage), Latency: resp.Latency}

	out, err := parse(resp.Message.Content)
	if err != nil {
		return partial, cost, err
	}
	out.Breaking, out.Dropped = ground(out.Breaking, sources)
	out.Excerpt, out.ExcerptSource = excerpt, excerptSource
	out.Module, out.From, out.To = in.Module, in.From, in.To
	out.Files = files
	out.Sources = urls
	out.FetchedAt = f.now().UTC()
	out.Search, out.Releases, out.ReleasesError = trace, partial.Releases, partial.ReleasesError
	return out, cost, nil
}

func nothingFound(trace *SearchTrace, releasesError string) error {
	var why []string
	if trace != nil && trace.Error != "" {
		why = append(why, "the search failed: "+trace.Error)
	}
	if trace != nil && trace.Skipped != "" {
		why = append(why, "the search was skipped: "+trace.Skipped)
	}
	if releasesError != "" {
		why = append(why, "the releases could not be read: "+releasesError)
	}
	if len(why) == 0 {
		return errors.New("no release notes or migration guide could be found")
	}
	return fmt.Errorf("no release notes or migration guide could be found (%s)", strings.Join(why, "; "))
}

func (f Forecaster) search(ctx context.Context, in Input) ([]Result, *SearchTrace) {
	if f.Search == nil {
		return nil, nil
	}
	trace := &SearchTrace{Provider: ProviderOf(f.Search), Query: Query(in.Module, in.From, in.To), At: f.now().UTC()}
	began := time.Now()
	results, err := f.Search.Search(ctx, trace.Query, maxSearchResults)
	var skip *SkipError
	if errors.As(err, &skip) {
		trace.Skipped = skip.Reason
		return nil, trace
	}
	trace.LatencyMS = time.Since(began).Milliseconds()
	trace.Micros = f.SearchMicros
	if err != nil {
		trace.Error = err.Error()
		return nil, trace
	}
	pages := slices.DeleteFunc(slices.Clone(results), func(r Result) bool { return !About(r.URL, in.Module) })
	for i, r := range pages {
		if text, _ := pageText(i, r); text != "" {
			trace.Kept = append(trace.Kept, r.URL)
		}
	}
	trace.Kept = dedupe(trace.Kept)
	for _, r := range results {
		trace.Results = append(trace.Results, SearchHit{Title: r.Title, URL: r.URL, Score: r.Score, Kept: slices.Contains(trace.Kept, r.URL)})
	}
	return pages, trace
}

func (f Forecaster) releases(ctx context.Context, in Input) ([]Release, error) {
	if f.Releases == nil {
		return nil, nil
	}
	list, err := f.Releases.Between(ctx, in.Module, in.From, in.To)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func releaseURLs(releases []Release) []string {
	urls := make([]string, 0, len(releases))
	for _, r := range releases {
		urls = append(urls, r.URL)
	}
	return dedupe(urls)
}

func pageText(i int, r Result) (string, int) {
	text, limit := CleanPage(r.RawContent), maxSourceChars
	if text == "" || i > 0 {
		text, limit = strings.TrimSpace(r.Content), maxExtraChars
	}
	return text, limit
}

func assemble(pages []Result, releases []Release) (text string, urls []string, excerpt, excerptSource string) {
	var b strings.Builder
	for i, r := range pages {
		page, limit := pageText(i, r)
		if page == "" {
			continue
		}
		fmt.Fprintf(&b, "\nSearch result: %s (%s)\n%s\n", r.Title, r.URL, truncate(page, limit))
		urls = append(urls, r.URL)
		if excerpt == "" && i == 0 {
			excerpt, excerptSource = truncate(page, maxExcerptChars), r.URL
		}
	}
	if len(releases) > 0 {
		b.WriteString("\nRelease notes, newest first:\n")
		for _, r := range releases {
			fmt.Fprintf(&b, "\n## %s\n%s\n", r.Version, truncate(strings.TrimSpace(r.Notes), maxReleaseChars))
			if r.URL != "" {
				urls = append(urls, r.URL)
			}
		}
	}
	return b.String(), dedupe(urls), excerpt, excerptSource
}

type CachedReleases struct {
	Inner Releases
	Cache fixture.Store
	TTL   time.Duration
	Now   func() time.Time
}

type releasesEntry struct {
	FetchedAt time.Time `json:"fetched_at"`
	Releases  []Release `json:"releases"`
}

func (c CachedReleases) Between(ctx context.Context, module, from, to string) ([]Release, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = ReleasesTTL
	}
	key, err := cacheKey(Input{Module: module, From: from, To: to})
	if err != nil {
		return nil, err
	}
	var hit releasesEntry
	stored := c.Cache != nil && c.Cache.Load(ReleasesKind, key, &hit) == nil
	if stored && now().Sub(hit.FetchedAt) < ttl {
		return hit.Releases, nil
	}
	list, err := c.Inner.Between(ctx, module, from, to)
	if err != nil {
		if stored {
			return hit.Releases, nil
		}
		return nil, err
	}
	if c.Cache != nil {
		_ = c.Cache.Save(ReleasesKind, key, releasesEntry{FetchedAt: now().UTC(), Releases: list})
	}
	return list, nil
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
	if err := json.Unmarshal([]byte(jsonfix.Repair(answer[start:end+1])), &doc); err != nil {
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
