package redis

import (
	"context"
	"fmt"
	"os"

	"github.com/rebornace/baize/internal/middleware"
	goredis "github.com/redis/go-redis/v9"
)

// Config configures the redis middleware driver.
type Config struct {
	Addr, Username, Password, Stream, ConsumerGroup, EventsChannel, ConsumerName string
	DB                                                                           int
}

// Open builds a redis-backed Middleware.
func Open(ctx context.Context, cfg Config) (*middleware.Middleware, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis middleware: addr is required")
	}
	if cfg.Stream == "" {
		cfg.Stream = "baize:runs"
	}
	if cfg.ConsumerGroup == "" {
		cfg.ConsumerGroup = "baize-workers"
	}
	if cfg.EventsChannel == "" {
		cfg.EventsChannel = "baize:run-events"
	}
	if cfg.ConsumerName == "" {
		name, _ := os.Hostname()
		if name == "" {
			name = "baize-worker"
		}
		cfg.ConsumerName = name
	}

	client := goredis.NewClient(&goredis.Options{
		Addr: cfg.Addr, DB: cfg.DB, Username: cfg.Username, Password: cfg.Password,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis middleware: connect: %w", err)
	}
	q := newQueue(client, cfg.Stream, cfg.ConsumerGroup, cfg.ConsumerName)
	if err := q.ensureGroup(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	b := newBus(client, cfg.EventsChannel)
	l := newLimiter(client)
	mw := &middleware.Middleware{Queue: q, Bus: b, Limiter: l}
	mw.Close = func() error {
		_ = b.Close()
		_ = q.Close()
		_ = client.Close()
		return nil
	}
	return mw, nil
}

func init() {
	middleware.RegisterDriver("redis", func(ctx context.Context, o middleware.Options) (*middleware.Middleware, error) {
		name, _ := os.Hostname()
		cfg := Config{
			Addr: o.Redis.Addr, DB: o.Redis.DB, Username: o.Redis.Username, Password: o.Redis.Password,
			Stream: o.Redis.Stream, ConsumerGroup: o.Redis.ConsumerGroup,
			EventsChannel: o.Redis.EventsChannel, ConsumerName: name,
		}
		return Open(ctx, cfg)
	})
}
