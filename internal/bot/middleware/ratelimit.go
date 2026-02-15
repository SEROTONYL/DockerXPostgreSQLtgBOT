// Package middleware — ratelimit.go ограничивает частоту запросов.
// Защищает от спама и DDoS-атак.
package middleware

import (
	"sync"
	"time"
)

// RateLimiter ограничивает количество запросов на пользователя.
// Использует алгоритм скользящего окна.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[int64][]time.Time // user_id → список временных меток
	limit    int                   // Макс. запросов
	window   time.Duration         // Временное окно
}

// NewRateLimiter создаёт rate-limiter.
//
// Параметры:
//   - limit: максимум запросов за окно (например, 10)
//   - window: длительность окна (например, 1 минута)
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[int64][]time.Time),
		limit:    limit,
		window:   window,
	}

	// Фоновая очистка старых записей каждые 5 минут
	go rl.cleanup()

	return rl
}

// Allow проверяет, может ли пользователь отправить ещё один запрос.
// Возвращает true если лимит не превышен.
func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Убираем устаревшие запросы
	var recent []time.Time
	for _, t := range rl.requests[userID] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	// Проверяем лимит
	if len(recent) >= rl.limit {
		rl.requests[userID] = recent
		return false
	}

	// Добавляем текущий запрос
	recent = append(recent, now)
	rl.requests[userID] = recent
	return true
}

// cleanup периодически удаляет старые записи.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
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
