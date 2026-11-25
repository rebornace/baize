package api

import (
	"context"
	"log"
	"sync"
	"time"
)

const costFinalizeBatch = 200
const costFinalizeMaxPerPass = 5000
const costFinalizeLookback = 90 * 24 * time.Hour
const defaultCostFinalizeInterval = 5 * time.Minute

var costFinalizeMu sync.Mutex

// finalizeMissingRunCosts persists cached costs for terminal runs that do not
// have a run_costs row yet. Scope totals then sum run_costs via SQL instead of
// walking each run, so busy agents/conversations are not capped by a scan.
func (s *Server) finalizeMissingRunCosts() (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	since := time.Now().UTC().Add(-costFinalizeLookback)
	total := 0
	for {
		ids, err := s.Store.ListRunIDsMissingCostsSince(since, costFinalizeBatch)
		if err != nil {
			return total, err
		}
		for _, id := range ids {
			rec, err := s.Store.GetRun(id)
			if err != nil || rec == nil {
				continue
			}
			s.runCostCached(rec)
			total++
		}
		if len(ids) < costFinalizeBatch || total >= costFinalizeMaxPerPass {
			break
		}
	}
	return total, nil
}

// StartCostFinalizer runs an immediate pass and then repeats until ctx ends.
func (s *Server) StartCostFinalizer(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultCostFinalizeInterval
	}
	run := func() {
		if !costFinalizeMu.TryLock() {
			return
		}
		defer costFinalizeMu.Unlock()
		n, err := s.finalizeMissingRunCosts()
		if err != nil {
			log.Printf("cost finalizer: %v", err)
		} else if n > 0 {
			log.Printf("cost finalizer: priced %d terminal runs", n)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
