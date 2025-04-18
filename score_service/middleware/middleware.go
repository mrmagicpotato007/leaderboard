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

type RequestValidationMiddleware struct {
	nonceStore map[string]time.Time
	mu         sync.RWMutex
}

func NewRequestValidationMiddleware() *RequestValidationMiddleware {
	return &RequestValidationMiddleware{
		nonceStore: make(map[string]time.Time),
	}
}

//to-do need to validate
func (rv *RequestValidationMiddleware) cleanupNonces() {
	rv.mu.Lock()
	defer rv.mu.Unlock()

	threshold := time.Now().Add(-5 * time.Minute)
	for nonce, timestamp := range rv.nonceStore {
		if timestamp.Before(threshold) {
			delete(rv.nonceStore, nonce)
		}
	}
}

func (rv *RequestValidationMiddleware) isNonceValid(nonce string) bool {
	rv.mu.Lock()
	defer rv.mu.Unlock()

	if _, exists := rv.nonceStore[nonce]; exists {
		return false
	}

	rv.nonceStore[nonce] = time.Now()
	return true
}

// nounncing related
// func (rv *RequestValidationMiddleware) ValidateRequestMiddleware(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		nonce := r.Header.Get("X-Request-Nonce")
// 		timestamp := r.Header.Get("X-Request-Timestamp")
// 		signature := r.Header.Get("X-Request-Signature")

// 		if nonce == "" || timestamp == "" || signature == "" {
// 			http.Error(w, "Missing request validation headers", http.StatusBadRequest)
// 			return
// 		}

// 		reqTime, err := time.Parse(time.RFC3339, timestamp)
// 		if err != nil || time.Since(reqTime) > 5*time.Minute {
// 			http.Error(w, "Invalid or expired request timestamp", http.StatusBadRequest)
// 			return
// 		}

// 		if !rv.isNonceValid(nonce) {
// 			http.Error(w, "Invalid or reused nonce", http.StatusBadRequest)
// 			return
// 		}

// 		if !validateRequestSignature(r, signature) {
// 			http.Error(w, "Invalid request signature", http.StatusBadRequest)
// 			return
// 		}

// 		next.ServeHTTP(w, r)
// 	})
// }

func validateRequestSignature(r *http.Request, signature string) bool {
	// to-do Implement HMAC validation using a shared secret
	return true
}
