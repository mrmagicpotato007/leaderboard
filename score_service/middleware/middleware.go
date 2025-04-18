package middleware

import (
	"log"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter implements rate limiting per user
type RateLimiter struct {
	limiters map[int]*rate.Limiter
	mu       sync.RWMutex
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[int]*rate.Limiter),
	}
}

func (rl *RateLimiter) getLimiter(userID int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[userID]
	if !exists {
		limiter = rate.NewLimiter(rate.Every(time.Minute/30), 1)
		rl.limiters[userID] = limiter
	}

	return limiter
}

func (rl *RateLimiter) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value("user_id").(int)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		limiter := rl.getLimiter(userID)
		log.Printf("Number of  tokens: %f for user %d ", limiter.Tokens(), userID)
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type NonceStore struct {
	nonces     map[string]time.Time
	mu         sync.RWMutex
	expiration time.Duration
}

func NewNonceStore(expiration time.Duration) *NonceStore {
	ns := &NonceStore{
		nonces:     make(map[string]time.Time),
		expiration: expiration,
	}

	go ns.startCleanupRoutine()

	return ns
}

func (ns *NonceStore) startCleanupRoutine() {
	ticker := time.NewTicker(ns.expiration / 2)
	defer ticker.Stop()

	for range ticker.C {
		ns.cleanup()
	}
}

func (ns *NonceStore) cleanup() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	now := time.Now()
	for nonce, expiry := range ns.nonces {
		if now.After(expiry) {
			delete(ns.nonces, nonce)
		}
	}
}

func (ns *NonceStore) IsValid(nonce string) bool {
	if nonce == "" {
		return false
	}

	ns.mu.Lock()
	defer ns.mu.Unlock()

	if _, exists := ns.nonces[nonce]; exists {
		return false
	}

	ns.nonces[nonce] = time.Now().Add(ns.expiration)
	return true
}

func (ns *NonceStore) IdempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.Header.Get("X-Request-ID")
		if nonce == "" {
			http.Error(w, "Missing request ID", http.StatusBadRequest)
			return
		}

		if !ns.IsValid(nonce) {
			http.Error(w, "Duplicate request", http.StatusConflict)
			return
		}

		next.ServeHTTP(w, r)
	})
}
