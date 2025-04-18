package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

const (
	kafkaTopic          = "game-sessions"
	kafkaGroupID        = "ranking-service"
	redisLeaderboardKey = "leaderboard:%s"
	topN                = 10
)

type GameSession struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Score     int       `json:"score"`
	GameMode  string    `json:"game_mode"`
	Timestamp time.Time `json:"timestamp"`
}

type KafkaMessage struct {
	EventType string      `json:"event_type"`
	Session   GameSession `json:"session"`
}

var (
	rdb       *redis.Client
	jwtSecret = []byte("secret-test")
)

func setupApplication() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
}

type LeaderboardEntry struct {
	UserName string  `json:"user_name"`
	UserID   string  `json:"user_id"`
	Score    float64 `json:"score"`
	Rank     int64   `json:"rank"`
}

func getTopHandler(w http.ResponseWriter, r *http.Request) {

	//hardcoded mode for now can be extended to support other modes from query params.
	leaderboardKey := getLeaderboardKey("classic")
	result, err := rdb.ZRevRangeWithScores(r.Context(), leaderboardKey, 0, topN-1).Result()
	if err != nil {
		log.Printf("failed to get leaderboard: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var entries []LeaderboardEntry
	var userIds []string
	for i, z := range result {
		userIds = append(userIds, strings.TrimPrefix(z.Member.(string), "user:"))
		entries = append(entries, LeaderboardEntry{
			UserID: strings.TrimPrefix(z.Member.(string), "user:"),
			Score:  z.Score,
			Rank:   int64(i + 1),
		})
	}

	// Get user names from user service
	userNames := getBatchUserInfo(userIds)

	// Add user names to entries
	for i := range entries {
		log.Println("user id", entries[i].UserID)
		if name, ok := userNames[entries[i].UserID]; ok {
			log.Println("user name", name)
			entries[i].UserName = name
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func getUserRankHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["userId"]

	//Hard coded to classic for now can be extended in future to support other modes.
	leaderboardKey := getLeaderboardKey("classic")
	playerKey := getUserKey(userID)

	// Get user's score
	score, err := rdb.ZScore(r.Context(), leaderboardKey, playerKey).Result()
	if err == redis.Nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("failed to get user score: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Get user's rank
	rank, err := rdb.ZRevRank(r.Context(), leaderboardKey, playerKey).Result()
	if err != nil {
		log.Printf("failed to get user rank: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	userNames := getBatchUserInfo([]string{userID})

	name := userNames[userID]
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LeaderboardEntry{
		UserName: name,
		UserID:   userID,
		Score:    score,
		Rank:     rank + 1,
	})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Remove 'Bearer ' prefix if present
		tokenString = strings.TrimPrefix(tokenString, "Bearer ")

		// Parse and validate the token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Extract claims and add to request context
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Add user ID to request context
			userID := fmt.Sprintf("%v", claims["user_id"])
			r = r.WithContext(context.WithValue(r.Context(), "user_id", userID))
		}

		next.ServeHTTP(w, r)
	})
}

func main() {

	setupApplication()
	// Setup HTTP routes
	r := mux.NewRouter()

	// Protected routes
	protected := r.PathPrefix("/v1").Subrouter()
	protected.Use(authMiddleware)

	// Add protected routes
	protected.HandleFunc("/leaderboard/top", getTopHandler).Methods("GET")
	protected.HandleFunc("/rank/{userId}", getUserRankHandler).Methods("GET")

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":8086",
		Handler: r,
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("ranking service starting on port 8086")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	<-sigChan
	log.Println("shutting down...")

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("failed to shutdown server: %v", err)
	}
}

func getLeaderboardKey(gameMode string) string {
	return "leaderboard:" + gameMode
}

func getUserKey(userID string) string {
	return fmt.Sprintf("user:%s", userID)
}

func getBatchUserInfo(userIDs []string) map[string]string {
	userIDMap := make(map[string]string)

	// Prepare request body
	body, err := json.Marshal(userIDs)
	if err != nil {
		log.Printf("Error marshaling user IDs: %v", err)
		return userIDMap
	}

	// Make request to user service
	resp, err := http.Post("http://localhost:8084/v1/UserInfo", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error calling user service: %v", err)
		return userIDMap
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("User service returned non-200 status: %d", resp.StatusCode)
		return userIDMap
	}

	var users []struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		log.Printf("Error decoding user service response: %v", err)
		return userIDMap
	}

	log.Println("recieved users", users)

	// Build map of user ID to username
	for _, user := range users {
		userIDMap[fmt.Sprintf("%d", user.ID)] = user.Username
	}

	return userIDMap
}
