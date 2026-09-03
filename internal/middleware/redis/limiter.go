package redis

import (
	"context"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// memberSeq makes ZSET members unique even when time.Now resolution is coarse
// (e.g. Windows ~ms), so rapid Allow calls do not collapse into one ZADD update.
var memberSeq atomic.Int64

const (
	defaultLimit  = 120
	defaultWindow = time.Minute
	keyPrefix     = "baize:rl:"
)

// Lua sliding-window: ZREMRANGEBYSCORE + ZCARD gate + ZADD + EXPIRE.
// KEYS[1]=zset key; ARGV[1]=now_ms ARGV[2]=window_ms ARGV[3]=limit ARGV[4]=member
var allowScript = goredis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local n = redis.call('ZCARD', key)
if n >= limit then
  return 0
end
redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window)
return 1
`)

type limiter struct {
	client *goredis.Client
	limit  int
	window time.Duration
}

func newLimiter(c *goredis.Client) *limiter {
	return &limiter{client: c, limit: defaultLimit, window: defaultWindow}
}

func (l *limiter) Allow(key string) bool {
	// Allow uses the driver default budget with the baize:rl: key prefix.
	return l.AllowBudget(keyPrefix+key, l.limit, l.window)
}

// AllowBudget evaluates a sliding-window budget for key (used by task 10 wrappers).
// Callers pass the full Redis key (e.g. "baize:rl:inbox:"+channelID). Fails open
// (returns true) on Redis errors.
func (l *limiter) AllowBudget(key string, limit int, window time.Duration) bool {
	nowMs := time.Now().UnixMilli()
	member := strconv.FormatInt(nowMs, 10) + ":" + strconv.FormatInt(memberSeq.Add(1), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n, err := allowScript.Run(ctx, l.client, []string{key},
		nowMs, window.Milliseconds(), limit, member,
	).Int()
	if err != nil {
		log.Printf("redis limiter: fail-open for key=%q: %v", key, err)
		return true
	}
	return n == 1
}
