package httpapi

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter — простое ограничение частоты по IP (фиксированное окно). Хватает для одного сервера;
// при нескольких — перенести в Redis.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*bucket
	now    func() time.Time
}

type bucket struct {
	start time.Time
	n     int
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, hits: map[string]*bucket{}, now: time.Now}
}

func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > 10000 { // чистка старых окон
		for k, b := range l.hits {
			if now.Sub(b.start) > l.window {
				delete(l.hits, k)
			}
		}
	}
	b, ok := l.hits[key]
	if !ok || now.Sub(b.start) > l.window {
		l.hits[key] = &bucket{start: now, n: 1}
		return true
	}
	b.n++
	return b.n <= l.limit
}

// Blocked — исчерпан ли лимит (без учёта новой попытки).
func (l *RateLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.hits[key]
	return ok && l.now().Sub(b.start) <= l.window && b.n >= l.limit
}

// Fail засчитывает неудачную попытку (например, неверный пароль).
func (l *RateLimiter) Fail(key string) { l.Allow(key) }

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(r.RemoteAddr) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "Слишком много попыток — подождите минуту", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
