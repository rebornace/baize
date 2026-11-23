package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadMiddlewareDefaults(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Middleware.Driver != "memory" {
		t.Fatalf("driver=%q want memory", cfg.Middleware.Driver)
	}
	if cfg.Middleware.WorkerConcurrency != 8 {
		t.Fatalf("workers=%d want 8", cfg.Middleware.WorkerConcurrency)
	}
	if cfg.Middleware.LeaseTTLSec != 60 {
		t.Fatalf("lease=%d want 60", cfg.Middleware.LeaseTTLSec)
	}
	if cfg.Middleware.ReconcileIntervalSec != 15 {
		t.Fatalf("reconcile=%d want 15", cfg.Middleware.ReconcileIntervalSec)
	}
	if cfg.Middleware.Redis.Stream != "baize:runs" {
		t.Fatalf("stream=%q want baize:runs", cfg.Middleware.Redis.Stream)
	}
	if cfg.Middleware.Redis.ConsumerGroup != "baize-workers" {
		t.Fatalf("consumer_group=%q want baize-workers", cfg.Middleware.Redis.ConsumerGroup)
	}
	if cfg.Middleware.Redis.EventsChannel != "baize:run-events" {
		t.Fatalf("events_channel=%q want baize:run-events", cfg.Middleware.Redis.EventsChannel)
	}
}

func TestLoadMiddlewareRedisExplicit(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nmiddleware:\n  driver: redis\n  worker_concurrency: 4\n  lease_ttl_sec: 90\n  reconcile_interval_sec: 20\n  redis:\n    addr: \"127.0.0.1:6379\"\n    db: 1\n    password_env: REDIS_PASS\n    stream: s1\n    consumer_group: g1\n    events_channel: e1\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Middleware
	if m.Driver != "redis" || m.Redis.Addr != "127.0.0.1:6379" || m.Redis.DB != 1 ||
		m.Redis.PasswordEnv != "REDIS_PASS" || m.Redis.Stream != "s1" ||
		m.Redis.ConsumerGroup != "g1" || m.Redis.EventsChannel != "e1" ||
		m.WorkerConcurrency != 4 || m.LeaseTTLSec != 90 || m.ReconcileIntervalSec != 20 {
		t.Fatalf("redis middleware config not parsed: %+v", m)
	}
}
