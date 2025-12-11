package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"trackr/internal/platform/database"
)

type RateLimiter struct {
	store *sync.Map // map[string]*Bucket
}

type Bucket struct {
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
	// We need to know when it was last accessed to clean it up
	lastAccess time.Time
}

var rateLimits = map[string]int{
	"redirect":  10000, // 10k redirects per minute per org
	"api_read":  1000,  // 1k API reads per minute
	"api_write": 100,   // 100 API writes per minute
	"analytics": 500,   // 500 analytics queries per minute
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		store: &sync.Map{},
	}

	// Start cleanup routine
	go rl.cleanupLoop()

	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		rl.store.Range(func(key, value interface{}) bool {
			bucket := value.(*Bucket)
			bucket.mu.Lock()
			// If not accessed in last 10 minutes, delete it
			if now.Sub(bucket.lastAccess) > 10*time.Minute {
				rl.store.Delete(key)
			}
			bucket.mu.Unlock()
			return true
		})
	}
}

func (rl *RateLimiter) Allow(key string, limit int) bool {
	now := time.Now()

	val, _ := rl.store.LoadOrStore(key, &Bucket{
		tokens:     limit,
		lastRefill: now,
		lastAccess: now,
	})

	bucket := val.(*Bucket)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.lastAccess = now

	// Refill bucket
	elapsed := now.Sub(bucket.lastRefill)

	// Rate is limit / 60 seconds
	refillRate := float64(limit) / 60.0
	refillTokens := int(elapsed.Seconds() * refillRate)

	if refillTokens > 0 {
		if bucket.tokens+refillTokens > limit {
			bucket.tokens = limit
		} else {
			bucket.tokens += refillTokens
		}
		bucket.lastRefill = now
	}

	// Check availability
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}

	return false
}

// Global rate limiter instance
var GlobalRateLimiter = NewRateLimiter()

func RateLimit(limitType string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var key string

			tenant, ok := r.Context().Value("tenant").(*database.TenantContext)
			if ok && tenant != nil {
				key = fmt.Sprintf("%s:%s", tenant.OrgID, limitType)
			} else {
				ip := r.RemoteAddr
				key = fmt.Sprintf("%s:%s", ip, limitType)
			}

			limit, ok := rateLimits[limitType]
			if !ok {
				limit = 100
			}

			if !GlobalRateLimiter.Allow(key, limit) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next(w, r)
		}
	}
}
