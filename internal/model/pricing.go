package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/aalvaropc/upkeep/internal/config"
)

type Price struct {
	InputPerMillion       float64 `json:"input_per_million"`
	OutputPerMillion      float64 `json:"output_per_million"`
	CachedInputPerMillion float64 `json:"cached_input_per_million"`
}

func (p Price) Cost(u Usage) int64 {
	cached := min(max(u.CachedPromptTokens, 0), u.PromptTokens)
	cachedRate := p.CachedInputPerMillion
	if cachedRate == 0 {
		cachedRate = p.InputPerMillion
	}
	micros := float64(u.PromptTokens-cached)*p.InputPerMillion +
		float64(cached)*cachedRate +
		float64(u.CompletionTokens)*p.OutputPerMillion
	return int64(micros + 0.5)
}

type PriceBook map[string]Price

func (b PriceBook) Cost(model string, u Usage) (int64, bool) {
	p, ok := b[model]
	if !ok {
		return 0, false
	}
	return p.Cost(u), true
}

type Meter struct {
	Prices   PriceBook
	Baseline string
}

type Charge struct {
	Micros         int64 `json:"micros"`
	BaselineMicros int64 `json:"baseline_micros"`
	Known          bool  `json:"known"`
}

func (m Meter) Charge(model string, u Usage) Charge {
	cost, ok := m.Prices.Cost(model, u)
	if !ok {
		return Charge{}
	}
	c := Charge{Micros: cost, Known: true}
	if base, ok := m.Prices.Cost(m.Baseline, u); ok {
		c.BaselineMicros = base
	}
	return c
}

func (c Charge) Add(o Charge) Charge {
	return Charge{
		Micros:         c.Micros + o.Micros,
		BaselineMicros: c.BaselineMicros + o.BaselineMicros,
		Known:          c.Known || o.Known,
	}
}

func (c Charge) SavedMicros() int64 { return c.BaselineMicros - c.Micros }

func ApplyOverrides(book PriceBook, models config.Models, prices config.Prices) PriceBook {
	out := make(PriceBook, len(book)+3)
	maps.Copy(out, book)
	for _, o := range []struct {
		model string
		price *config.Price
	}{{models.Ultra, prices.Ultra}, {models.Super, prices.Super}, {models.Nano, prices.Nano}} {
		if o.price != nil {
			prev := out[o.model]
			prev.InputPerMillion = o.price.Input
			prev.OutputPerMillion = o.price.Output
			out[o.model] = prev
		}
	}
	return out
}

type catalogEntry struct {
	ID      string `json:"id"`
	Pricing struct {
		Prompt         string  `json:"prompt"`
		Completion     string  `json:"completion"`
		InputCacheRead *string `json:"input_cache_read"`
	} `json:"pricing"`
}

func ParseCatalog(raw []byte) (PriceBook, error) {
	var doc struct {
		Data []catalogEntry `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("the model catalog is not valid JSON: %w", err)
	}
	book := make(PriceBook, len(doc.Data))
	for _, e := range doc.Data {
		in, errIn := perMillion(e.Pricing.Prompt)
		out, errOut := perMillion(e.Pricing.Completion)
		if e.ID == "" || errIn != nil || errOut != nil || (in == 0 && out == 0) {
			continue
		}
		p := Price{InputPerMillion: in, OutputPerMillion: out}
		if e.Pricing.InputCacheRead != nil {
			if cached, err := perMillion(*e.Pricing.InputCacheRead); err == nil {
				p.CachedInputPerMillion = cached
			}
		}
		book[e.ID] = p
	}
	if len(book) == 0 {
		return nil, errors.New("the model catalog has no priced models")
	}
	return book, nil
}

func perMillion(perToken string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(perToken), 64)
	if err != nil {
		return 0, err
	}
	return v * 1e6, nil
}

func (t *TokenFactory) FetchPrices(ctx context.Context) (PriceBook, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.BaseURL+"/models?verbose=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Key.Reveal())
	req.Header.Set("Accept", "application/json")
	client := t.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, sanitize(err, t.Key)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, sanitize(err, t.Key)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{Status: resp.StatusCode, Message: errorMessage(raw[:min(len(raw), maxErrorBody)])}
	}
	return ParseCatalog(raw)
}
