package redis

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	goredis "github.com/redis/go-redis/v9"
)

// Config configures the redis middleware driver.
type Config struct {
	Addr, Username, Password, Stream, ConsumerGroup, EventsChannel, ConsumerName string
	DB                                                                           int
	// ConsumeBlock is the XReadGroup BLOCK duration; zero means 5s.
	ConsumeBlock time.Duration
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
		cfg.ConsumerName = defaultConsumerName()
	}

	client := goredis.NewClient(&goredis.Options{
		Addr: cfg.Addr, DB: cfg.DB, Username: cfg.Username, Password: cfg.Password,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis middleware: connect: %w", err)
	}
	q := newQueue(client, cfg.Stream, cfg.ConsumerGroup, cfg.ConsumerName, cfg.ConsumeBlock)
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

// defaultConsumerName returns hostname-pid (or baize-pid when hostname is empty)
// so multiple processes on one host do not share a single Streams consumer identity.
func defaultConsumerName() string {
	name, _ := os.Hostname()
	if name == "" {
		name = "baize"
	}
	return fmt.Sprintf("%s-%d", name, os.Getpid())
}

func init() {
	middleware.RegisterDriver("redis", func(ctx context.Context, o middleware.Options) (*middleware.Middleware, error) {
		// ConsumerName left empty so Open applies hostname-pid default.
		cfg := Config{
			Addr: o.Redis.Addr, DB: o.Redis.DB, Username: o.Redis.Username, Password: o.Redis.Password,
			Stream: o.Redis.Stream, ConsumerGroup: o.Redis.ConsumerGroup,
			EventsChannel: o.Redis.EventsChannel,
		}
		return Open(ctx, cfg)
	})
}
