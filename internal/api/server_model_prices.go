package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rebornace/baize/internal/pricing"
)

func (s *Server) modelPriceTable() pricing.Table {
	if s.Store != nil {
		if raw, ok, err := s.Store.GetSetting(pricing.SettingKey); err == nil && ok {
			if t, perr := pricing.Parse(raw); perr == nil {
				return t
			}
		}
	}
	return pricing.Default()
}

func (s *Server) handleGetModelPrices(w http.ResponseWriter, r *http.Request) {
	t := s.modelPriceTable()
	writeJSON(w, http.StatusOK, map[string]any{
		"currency": t.Currency,
		"source":   t.Source,
		"entries":  t.Entries,
		"defaults": pricing.Default(),
	})
}

func (s *Server) handlePutModelPrices(w http.ResponseWriter, r *http.Request) {
	var t pricing.Table
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	t.Normalize()
	if err := t.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if t.Source == "" {
		t.Source = "自定义价格 · " + time.Now().UTC().Format("2006-01-02")
	}
	raw, err := json.Marshal(t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(pricing.SettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	// Cached run/day costs were computed with the old prices.
	_ = s.Store.ClearCostCache()
	costCache.mu.Lock()
	costCache.at = time.Time{}
	costCache.mu.Unlock()
	writeJSON(w, http.StatusOK, t)
}
