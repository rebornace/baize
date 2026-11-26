package bootstrap

import (
	"runtime"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/metricsx"
)

// registerOpsMetrics exposes process, DB pool, SSE and replica liveness as
// Prometheus gauges. Each replica scrapes its own values; Prometheus adds the
// instance label externally.
func registerOpsMetrics(mx *metricsx.Collector, srv *api.Server) {
	mx.RegisterGauge("baize_goroutines", "Process goroutines.", func() float64 {
		return float64(runtime.NumGoroutine())
	})
	mx.RegisterGauge("baize_db_max_open_connections", "database/sql max open connections.", func() float64 {
		return float64(srv.PoolStats().MaxOpen)
	})
	mx.RegisterGauge("baize_db_open_connections", "database/sql open connections.", func() float64 {
		return float64(srv.PoolStats().Open)
	})
	mx.RegisterGauge("baize_db_in_use_connections", "database/sql in-use connections.", func() float64 {
		return float64(srv.PoolStats().InUse)
	})
	mx.RegisterGauge("baize_db_idle_connections", "database/sql idle connections.", func() float64 {
		return float64(srv.PoolStats().Idle)
	})
	mx.RegisterGauge("baize_db_wait_count", "Cumulative database/sql connection waits.", func() float64 {
		return float64(srv.PoolStats().WaitCount)
	})
	mx.RegisterGauge("baize_sse_streams_live", "Live pending-approval SSE streams.", func() float64 {
		live, _, _ := srv.PendingStreamStats()
		return float64(live)
	})
	mx.RegisterGauge("baize_sse_streams_dropped_total", "Cumulative dropped pending-approval SSE streams.", func() float64 {
		_, dropped, _ := srv.PendingStreamStats()
		return float64(dropped)
	})
	mx.RegisterGauge("baize_sse_streams_slow_total", "Cumulative slow pending-approval SSE streams.", func() float64 {
		_, _, slow := srv.PendingStreamStats()
		return float64(slow)
	})
	mx.RegisterGauge("baize_replicas_live", "Fresh replica heartbeats including this replica.", func() float64 {
		return float64(srv.LiveReplicaCount())
	})
}
