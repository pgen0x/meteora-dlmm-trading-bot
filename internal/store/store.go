package store

import (
	"context"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// Seen tracks which pools have already been emitted, so each qualifying pool
// fires exactly once until its TTL lapses. Backed by Redis when configured,
// otherwise an in-memory map (single-instance).
type Seen struct {
	rdb    *redis.Client
	key    string
	ttl    time.Duration
	mu     sync.Mutex
	mem    map[string]time.Time
}

// New builds a Seen store. addr == "" selects the in-memory backend.
func New(addr, key string, ttl time.Duration) *Seen {
	s := &Seen{key: key, ttl: ttl, mem: make(map[string]time.Time)}
	if addr != "" {
		s.rdb = redis.NewClient(&redis.Options{Addr: addr})
	}
	return s
}

// MarkIfNew atomically records id and reports whether it was newly added
// (true == first time we've seen it, caller should emit a signal).
func (s *Seen) MarkIfNew(ctx context.Context, id string) (bool, error) {
	if s.rdb != nil {
		added, err := s.rdb.SAdd(ctx, s.key, id).Result()
		if err != nil {
			return false, err
		}
		// Refresh TTL on the set each write (rolling window).
		s.rdb.Expire(ctx, s.key, s.ttl)
		return added == 1, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// Lazy expiry of the in-memory map.
	for k, t := range s.mem {
		if now.Sub(t) > s.ttl {
			delete(s.mem, k)
		}
	}
	if _, ok := s.mem[id]; ok {
		return false, nil
	}
	s.mem[id] = now
	return true, nil
}

// Unmark removes id from the seen set so a failed emit can retry on the next
// poll. Called when webhook delivery fails after MarkIfNew already recorded it.
func (s *Seen) Unmark(ctx context.Context, id string) {
	if s.rdb != nil {
		s.rdb.SRem(ctx, s.key, id)
		return
	}
	s.mu.Lock()
	delete(s.mem, id)
	s.mu.Unlock()
}
