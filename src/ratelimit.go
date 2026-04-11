package main

import (
	"slices"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	counts   map[int64][]time.Time
	limit    int
	interval time.Duration
}

func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		counts:   make(map[int64][]time.Time),
		limit:    limit,
		interval: interval,
	}
}

func (rl *RateLimiter) Allow(userID int64) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.interval)

	// Filter out old timestamps
	rl.counts[userID] = slices.DeleteFunc(rl.counts[userID], func(t time.Time) bool {
		return !t.After(cutoff)
	})
	current := rl.counts[userID]

	if len(current) >= rl.limit {
		// The cooldown ends when the oldest timestamp in 'current' falls out of the window
		cooldown := time.Until(current[0].Add(rl.interval))
		if cooldown < 0 {
			cooldown = 0
		}
		return false, cooldown
	}

	rl.counts[userID] = append(current, now)
	return true, 0
}
