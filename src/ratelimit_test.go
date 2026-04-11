package main

import (
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	limit := 3
	interval := 100 * time.Millisecond
	rl := NewRateLimiter(limit, interval)
	userID := int64(123)

	// First 'limit' calls should be allowed
	for i := 0; i < limit; i++ {
		allowed, cooldown := rl.Allow(userID)
		if !allowed {
			t.Errorf("expected call %d to be allowed", i+1)
		}
		if cooldown != 0 {
			t.Errorf("expected call %d to have 0 cooldown, got %v", i+1, cooldown)
		}
	}

	// The next call should be denied
	allowed, cooldown := rl.Allow(userID)
	if allowed {
		t.Error("expected call to be denied after reaching limit")
	}
	if cooldown <= 0 {
		t.Errorf("expected positive cooldown, got %v", cooldown)
	}

	// Wait for the interval to pass
	time.Sleep(interval + 10*time.Millisecond)

	// Next call should be allowed again
	allowed, cooldown = rl.Allow(userID)
	if !allowed {
		t.Error("expected call to be allowed after interval passed")
	}
	if cooldown != 0 {
		t.Errorf("expected 0 cooldown after interval passed, got %v", cooldown)
	}
}

func TestRateLimiter_MultipleUsers(t *testing.T) {
	limit := 1
	interval := 100 * time.Millisecond
	rl := NewRateLimiter(limit, interval)

	user1 := int64(1)
	user2 := int64(2)

	// User 1 consumes their limit
	allowed, _ := rl.Allow(user1)
	if !allowed {
		t.Fatal("user 1 first call failed")
	}

	// User 2 should still be allowed
	allowed, _ = rl.Allow(user2)
	if !allowed {
		t.Fatal("user 2 first call failed")
	}

	// User 1 should be denied
	allowed, _ = rl.Allow(user1)
	if allowed {
		t.Error("user 1 should be denied")
	}

	// User 2 should be denied
	allowed, _ = rl.Allow(user2)
	if allowed {
		t.Error("user 2 should be denied")
	}
}
