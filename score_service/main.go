package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"scoreservice/middleware"

	"scoreservice/models"

	"github.com/gocql/gocql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/segmentio/kafka-go"
)

const (
	MaxRequestSize = 1024
)

// KafkaMessage represents a message to be sent to Kafka
type KafkaMessage struct {
	Key   []byte
	Value []byte
}

var (
	jwtSecret        = []byte("secret-test")
	kafkaWriter      *kafka.Writer
	
	// Channel for sending messages to Kafka
	kafkaMessageChan chan KafkaMessage

	// Prometheus metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "score_service_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "score_service_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	kafkaWriteDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "score_service_kafka_write_duration_seconds",
			Help:    "Duration of Kafka write operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	kafkaWriteErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "score_service_kafka_write_errors_total",
			Help: "Total number of Kafka write errors",
		},
	)

	gameModeCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "score_service_game_mode_total",
			Help: "Total number of scores submitted by game mode",
		},
		[]string{"game_mode"},
	)
)

// startKafkaWorker initializes a background worker that consumes messages from the channel
// and publishes them to Kafka with retries and metrics
func startKafkaWorker(ctx context.Context) {
	log.Println("Starting Kafka background worker")
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Shutting down Kafka background worker")
				return
			case msg := <-kafkaMessageChan:
				// Start timing Kafka write
				kafkaTimer := prometheus.NewTimer(kafkaWriteDuration)
				
				// Try to publish with retries
				var writeErr error
				for retries := 0; retries < 3; retries++ {
					// Create a timeout context for this specific write
					writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					
					writeErr = kafkaWriter.WriteMessages(writeCtx, kafka.Message{
						Key:   msg.Key,
						Value: msg.Value,
					})
					
					cancel() // Always cancel the context to release resources
					
					if writeErr == nil {
						break
					}
					
					log.Printf("retry %d: Failed to write to Kafka: %v", retries+1, writeErr)
					time.Sleep(time.Second * time.Duration(retries+1))
				}
				
				kafkaTimer.ObserveDuration() // Stop the timer
				
				if writeErr != nil {
					log.Printf("failed to write to Kafka after retries: %v", writeErr)
					kafkaWriteErrors.Inc()
				}
			}
		}
	}()
}

func setUpApplication() {
	// Initialize Kafka message channel with buffer
	kafkaMessageChan = make(chan KafkaMessage, 100)
	
	// Initialize Kafka writer
	kafkaWriter = kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "game-sessions",
	})
}

func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint := r.URL.Path
		method := r.Method

		// Create a custom response writer to capture the status code
		ww := middleware.NewResponseWriter(w)

		// Start the timer
		timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(method, endpoint))

		// Call the next handler
		next.ServeHTTP(ww, r)

		// Stop the timer
		timer.ObserveDuration()

		// Record the request
		status := fmt.Sprintf("%d", ww.Status())
		httpRequestsTotal.WithLabelValues(method, endpoint, status).Inc()
	})
}

func main() {
	setUpApplication()
	defer kafkaWriter.Close()
	
	// Create a context that will be canceled when the application shuts down
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the Kafka background worker
	startKafkaWorker(ctx)

	rateLimiter := middleware.NewRateLimiter()
	//requestValidator := middleware.NewRequestValidationMiddleware()

	metricsRouter := http.NewServeMux()
	metricsRouter.Handle("/metrics", promhttp.Handler())

	// Create the main API router with all middleware
	apiRouter := mux.NewRouter()
	apiRouter.Use(prometheusMiddleware)
	apiRouter.Use(authMiddleware)
	apiRouter.Use(rateLimiter.RateLimitMiddleware)
	//apiRouter.Use(requestValidator.ValidateRequestMiddleware)

	apiRouter.Use(securityHeadersMiddleware)
	apiRouter.HandleFunc("/v1/score", createScoreHandler).Methods("POST")

	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			metricsRouter.ServeHTTP(w, r)
		} else {
			apiRouter.ServeHTTP(w, r)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}

	log.Printf("Score service starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mainHandler))
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}

		userIDValue, exists := claims["user_id"]
		if !exists {
			http.Error(w, "User ID missing in token", http.StatusUnauthorized)
			return
		}

		userIDFloat, ok := userIDValue.(float64)
		if !ok {
			http.Error(w, "Invalid user ID format", http.StatusUnauthorized)
			return
		}

		userID := int(userIDFloat)
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// to-do need to read more about these headers
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

func createScoreHandler(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from JWT token
	userID := r.Context().Value("user_id").(int)

	// Decode the request body
	var session models.GameSession
	err := json.NewDecoder(r.Body).Decode(&session)
	if err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	err = session.ValidateSession()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session.UserID = fmt.Sprintf("%d", userID)
	session.SessionID = gocql.TimeUUID()
	session.Timestamp = session.SessionID.Time()

	// Increment game mode counter
	gameModeCounter.WithLabelValues(session.GameMode).Inc()

	sessionJSON, err := json.Marshal(map[string]interface{}{
		"event_type": "game_score_recorded",
		"session":    session,
	})

	if err != nil {
		log.Printf("Error marshaling session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send the message to the Kafka channel for async processing
	// This won't block the HTTP handler
	select {
	case kafkaMessageChan <- KafkaMessage{
		Key:   []byte(session.UserID),
		Value: sessionJSON,
	}:
		// Message successfully queued
		log.Printf("Message for user %s queued for Kafka processing", session.UserID)
	default:
		// Channel buffer is full, log the error but still return success to client
		// This prevents the HTTP handler from blocking when the channel is full
		log.Printf("WARNING: Kafka message channel full, dropping message for user %s", session.UserID)
		kafkaWriteErrors.Inc()
	}

	json.NewEncoder(w).Encode(session)
}
