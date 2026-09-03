package memory

import (
	"context"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
)

func init() {
	middleware.RegisterDriver("memory", func(_ context.Context, _ middleware.Options) (*middleware.Middleware, error) {
		q := newQueue()
		b := newBus(eventbus.NewHub())
		l := newLimiter()
		return &middleware.Middleware{
			Queue:   q,
			Bus:     b,
			Limiter: l,
			Close: func() error {
				_ = q.Close()
				_ = b.Close()
				return nil
			},
		}, nil
	})
}

func nowUTC() time.Time { return time.Now().UTC() }
