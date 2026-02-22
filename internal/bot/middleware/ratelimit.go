package middleware

import (
	"sync"
	"time"
)

// RateLimiter ограничивает количество запросов на пользователя.
// Использует алгоритм скользящего окна.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[int64][]time.Time
	limit    int
	window   time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[int64][]time.Time),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Close останавливает фоновую горутину очистки.
// Его надо вызывать на shutdown (иначе cleanup будет жить вечно).
func (rl *RateLimiter) Close() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var recent []time.Time
	for _, t := range rl.requests[userID] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.limit {
		rl.requests[userID] = recent
		return false
	}

	recent = append(recent, now)
	rl.requests[userID] = recent
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.window)
			for userID, times := range rl.requests {
				var recent []time.Time
				for _, t := range times {
					if t.After(cutoff) {
						recent = append(recent, t)
					}
				}
				if len(recent) == 0 {
					delete(rl.requests, userID)
				} else {
					rl.requests[userID] = recent
				}
			}
			rl.mu.Unlock()
		}
	}
}