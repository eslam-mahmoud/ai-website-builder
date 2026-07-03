// Package cache provides a small key-value cache used for preview tokens,
// rate limiting, and deployment locks. Redis backs it when REDIS_ADDR is
// configured; otherwise an in-process store is used. PostgreSQL remains the
// source of truth for all persistent data.
package cache

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache interface {
	Get(ctx context.Context, key string) (string, bool)
	Set(ctx context.Context, key, value string, ttl time.Duration)
	Delete(ctx context.Context, key string)
	// SetNX sets key only if absent, returning true on acquisition. Used
	// for per-website deployment locks.
	SetNX(ctx context.Context, key, value string, ttl time.Duration) bool
	// Incr increments a counter with ttl applied on first increment. Used
	// for rate limiting.
	Incr(ctx context.Context, key string, ttl time.Duration) int64
}

// --- Redis implementation ---

type redisCache struct{ c *redis.Client }

func NewRedis(addr string) (Cache, error) {
	c := redis.NewClient(&redis.Options{Addr: addr})
	if err := c.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &redisCache{c: c}, nil
}

func (r *redisCache) Get(ctx context.Context, key string) (string, bool) {
	v, err := r.c.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return v, true
}

func (r *redisCache) Set(ctx context.Context, key, value string, ttl time.Duration) {
	r.c.Set(ctx, key, value, ttl)
}

func (r *redisCache) Delete(ctx context.Context, key string) { r.c.Del(ctx, key) }

func (r *redisCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) bool {
	ok, _ := r.c.SetNX(ctx, key, value, ttl).Result()
	return ok
}

func (r *redisCache) Incr(ctx context.Context, key string, ttl time.Duration) int64 {
	n, err := r.c.Incr(ctx, key).Result()
	if err != nil {
		return 0
	}
	if n == 1 {
		r.c.Expire(ctx, key, ttl)
	}
	return n
}

// --- In-memory implementation ---

type memEntry struct {
	value     string
	expiresAt time.Time
}

type memCache struct {
	mu sync.Mutex
	m  map[string]memEntry
}

func NewMemory() Cache {
	mc := &memCache{m: make(map[string]memEntry)}
	go mc.janitor()
	return mc
}

func (m *memCache) janitor() {
	for range time.Tick(time.Minute) {
		now := time.Now()
		m.mu.Lock()
		for k, e := range m.m {
			if now.After(e.expiresAt) {
				delete(m.m, k)
			}
		}
		m.mu.Unlock()
	}
}

func (m *memCache) get(key string) (memEntry, bool) {
	e, ok := m.m[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(m.m, key)
		return memEntry{}, false
	}
	return e, true
}

func (m *memCache) Get(_ context.Context, key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.get(key)
	return e.value, ok
}

func (m *memCache) Set(_ context.Context, key, value string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = memEntry{value: value, expiresAt: time.Now().Add(ttl)}
}

func (m *memCache) Delete(_ context.Context, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}

func (m *memCache) SetNX(_ context.Context, key, value string, ttl time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.get(key); ok {
		return false
	}
	m.m[key] = memEntry{value: value, expiresAt: time.Now().Add(ttl)}
	return true
}

func (m *memCache) Incr(_ context.Context, key string, ttl time.Duration) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.get(key)
	if !ok {
		m.m[key] = memEntry{value: "1", expiresAt: time.Now().Add(ttl)}
		return 1
	}
	n, _ := strconv.ParseInt(e.value, 10, 64)
	n++
	e.value = strconv.FormatInt(n, 10)
	m.m[key] = e
	return n
}
