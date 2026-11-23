package pricing

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// SettingKey stores the operator-edited price table.
const SettingKey = "model_prices"

//go:embed default_prices.json
var defaultJSON []byte

// Entry is a per-model price in USD per 1,000,000 tokens.
type Entry struct {
	Model              string  `json:"model"`
	InputPerMTok       float64 `json:"input_per_mtok"`
	OutputPerMTok      float64 `json:"output_per_mtok"`
	CachedInputPerMTok float64 `json:"cached_input_per_mtok"`
}

// Table is the effective price list. Source is a human note, not a machine key.
type Table struct {
	Currency string  `json:"currency"`
	Source   string  `json:"source,omitempty"`
	Entries  []Entry `json:"entries"`
}

// Default returns the bundled example table.
func Default() Table {
	var t Table
	_ = json.Unmarshal(defaultJSON, &t)
	return t
}

// Parse validates a table from JSON.
func Parse(raw []byte) (Table, error) {
	var t Table
	if err := json.Unmarshal(raw, &t); err != nil {
		return Table{}, fmt.Errorf("pricing: invalid json: %w", err)
	}
	if err := t.Validate(); err != nil {
		return Table{}, err
	}
	return t, nil
}

// Validate checks model names and non-negative prices.
func (t Table) Validate() error {
	if len(t.Entries) == 0 {
		return fmt.Errorf("pricing: at least one entry required")
	}
	seen := map[string]bool{}
	for i, e := range t.Entries {
		m := strings.TrimSpace(e.Model)
		if m == "" {
			return fmt.Errorf("pricing: entry %d: model required", i+1)
		}
		if seen[strings.ToLower(m)] {
			return fmt.Errorf("pricing: duplicate model %q", m)
		}
		seen[strings.ToLower(m)] = true
		if e.InputPerMTok < 0 || e.OutputPerMTok < 0 || e.CachedInputPerMTok < 0 {
			return fmt.Errorf("pricing: model %q: prices must not be negative", m)
		}
		if e.InputPerMTok > 1_000_000 || e.OutputPerMTok > 1_000_000 || e.CachedInputPerMTok > 1_000_000 {
			return fmt.Errorf("pricing: model %q: price too large", m)
		}
	}
	return nil
}

// Normalize trims names and defaults the currency.
func (t *Table) Normalize() {
	if strings.TrimSpace(t.Currency) == "" {
		t.Currency = "USD"
	}
	for i := range t.Entries {
		t.Entries[i].Model = strings.TrimSpace(t.Entries[i].Model)
	}
}

// Lookup finds the longest case-insensitive prefix match, so versioned or
// provider-qualified model names still match a base entry.
func (t Table) Lookup(model string) (Entry, bool) {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return Entry{}, false
	}
	best := -1
	var out Entry
	for _, e := range t.Entries {
		key := strings.ToLower(strings.TrimSpace(e.Model))
		if key == "" || !strings.HasPrefix(name, key) {
			continue
		}
		if len(key) > best {
			best = len(key)
			out = e
		}
	}
	return out, best >= 0
}

// Cost returns the estimated USD cost for one run's aggregate usage.
func (t Table) Cost(model string, prompt, completion, cached int) (float64, Entry, bool) {
	e, ok := t.Lookup(model)
	if !ok {
		return 0, Entry{}, false
	}
	if cached < 0 {
		cached = 0
	}
	if cached > prompt {
		cached = prompt
	}
	cachedPrice := e.CachedInputPerMTok
	if cachedPrice <= 0 {
		cachedPrice = e.InputPerMTok
	}
	uncached := prompt - cached
	cost := float64(uncached)*e.InputPerMTok/1_000_000 +
		float64(cached)*cachedPrice/1_000_000 +
		float64(completion)*e.OutputPerMTok/1_000_000
	return cost, e, true
}
