package api

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const replicaStatusPrefix = "replica_status:"
const replicaStaleAfter = 60 * time.Second
const defaultReplicaHeartbeatInterval = 15 * time.Second

type replicaStreamStats struct {
	Live    int64 `json:"live"`
	Dropped int64 `json:"dropped"`
	Slow    int64 `json:"slow"`
}

type replicaHeartbeat struct {
	ID         string             `json:"id"`
	At         time.Time          `json:"at"`
	Goroutines int                `json:"goroutines"`
	DB         store.PoolStats    `json:"db"`
	Streams    replicaStreamStats `json:"streams"`
}

func (s *Server) replicaID() string {
	if v := strings.TrimSpace(os.Getenv("BAIZE_REPLICA_ID")); v != "" {
		return v
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "replica"
	}
	return host
}

func (s *Server) currentReplicaHeartbeat(now time.Time) replicaHeartbeat {
	db := s.PoolStats()
	live, dropped, slow := s.pendingStreamStats()
	return replicaHeartbeat{
		ID: s.replicaID(), At: now, Goroutines: runtime.NumGoroutine(), DB: db,
		Streams: replicaStreamStats{Live: live, Dropped: dropped, Slow: slow},
	}
}

// PoolStats exposes the active store's connection pool for metrics.
func (s *Server) PoolStats() store.PoolStats {
	if s.Store == nil {
		return store.PoolStats{}
	}
	if ps, ok := s.Store.(store.PoolStatter); ok {
		return ps.PoolStats()
	}
	return store.PoolStats{}
}

// PendingStreamStats exports SSE stream counters for metrics.
func (s *Server) PendingStreamStats() (live, dropped, slow int64) {
	return s.pendingStreamStats()
}

// LiveReplicaCount counts fresh replica heartbeats, including this one.
func (s *Server) LiveReplicaCount() int {
	return len(s.liveReplicas(time.Now().UTC()))
}

func (s *Server) writeReplicaHeartbeat(now time.Time) {
	if s.Store == nil {
		return
	}
	hb := s.currentReplicaHeartbeat(now)
	raw, _ := json.Marshal(hb)
	if err := s.Store.UpsertSetting(replicaStatusPrefix+hb.ID, raw); err != nil {
		log.Printf("replica heartbeat: %v", err)
	}
}

// liveReplicas merges fresh heartbeats with this replica's live numbers.
func (s *Server) liveReplicas(now time.Time) []replicaHeartbeat {
	out := map[string]replicaHeartbeat{}
	if s.Store != nil {
		if settings, err := s.Store.ListSettingsByPrefix(replicaStatusPrefix); err == nil {
			for _, raw := range settings {
				var hb replicaHeartbeat
				if json.Unmarshal(raw, &hb) != nil || hb.ID == "" {
					continue
				}
				if now.Sub(hb.At) > replicaStaleAfter {
					continue
				}
				out[hb.ID] = hb
			}
		}
	}
	current := s.currentReplicaHeartbeat(now)
	out[current.ID] = current
	list := make([]replicaHeartbeat, 0, len(out))
	for _, hb := range out {
		list = append(list, hb)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// StartReplicaHeartbeat publishes this replica's process stats for the
// multi-replica ops view. Stale entries are ignored by readers.
func (s *Server) StartReplicaHeartbeat(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultReplicaHeartbeatInterval
	}
	s.writeReplicaHeartbeat(time.Now().UTC())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.writeReplicaHeartbeat(time.Now().UTC())
		}
	}
}
