// Package ratelimit counts attempts per key in fixed windows.
//
// With Redis every auth pod shares the counts. Without it (or while Redis is
// unreachable) each pod counts on its own, which still slows a brute-force
// attempt down, just less precisely.
package ratelimit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	// Allow records one attempt and reports whether it is within limit.
	Allow(ctx context.Context, key string, limit int, window time.Duration) bool
}

// New returns a Redis-backed limiter when url is set, else an in-memory one.
func New(url string, log *slog.Logger) (Limiter, func(), error) {
	mem := NewMemory()
	if url == "" {
		return mem, func() {}, nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, nil, err
	}
	client := redis.NewClient(opts)
	return &redisLimiter{client: client, fallback: mem, log: log}, func() { _ = client.Close() }, nil
}

type redisLimiter struct {
	client   *redis.Client
	fallback *Memory
	log      *slog.Logger
}

func (r *redisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	slot := time.Now().UnixNano() / int64(window)
	k := "hayat:rl:" + key + ":" + itoa(slot)
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, window)
	if _, err := pipe.Exec(ctx); err != nil {
		r.log.Warn("rate limit: redis unavailable, counting in memory", "err", err)
		return r.fallback.Allow(ctx, key, limit, window)
	}
	return incr.Val() <= int64(limit)
}

// Memory is a per-process limiter.
type Memory struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	sweep   time.Time
}

type bucket struct {
	start time.Time
	count int
}

func NewMemory() *Memory { return &Memory{buckets: map[string]*bucket{}} }

func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if now.Sub(m.sweep) > time.Minute {
		for k, b := range m.buckets {
			if now.Sub(b.start) > time.Hour {
				delete(m.buckets, k)
			}
		}
		m.sweep = now
	}
	b, ok := m.buckets[key]
	if !ok || now.Sub(b.start) >= window {
		b = &bucket{start: now}
		m.buckets[key] = b
	}
	b.count++
	return b.count <= limit
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
