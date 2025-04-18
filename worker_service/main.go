package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

const (
	kafkaTopic   = "game-sessions"
	kafkaGroupID = "worker-service"
	kafkaServer  = "localhost:9092"
	redisServer  = "localhost:6379"
)

var (
	messagesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "worker_messages_processed_total",
		Help: "The total number of processed messages",
	})

	messageProcessingErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "worker_message_processing_errors_total",
		Help: "The total number of message processing errors",
	})

	messageProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "worker_message_processing_duration_seconds",
		Help:    "The duration of message processing in seconds",
		Buckets: prometheus.DefBuckets,
	})

	storageWriteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "worker_storage_write_duration_seconds",
		Help:    "The duration of storage write operations in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"storage_type"})

	storageWriteErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "worker_storage_write_errors_total",
		Help: "The total number of storage write errors",
	}, []string{"storage_type"})

	gameModeCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "worker_game_mode_total",
		Help: "The total number of processed games by mode",
	}, []string{"game_mode"})
)

type GameSession struct {
	SessionID gocql.UUID `json:"session_id"`
	UserID    string     `json:"user_id"`
	Score     int        `json:"score"`
	GameMode  string     `json:"game_mode"`
	Timestamp time.Time  `json:"timestamp"`
}

type KafkaMessage struct {
	EventType string      `json:"event_type"`
	Session   GameSession `json:"session"`
}

type StorageWriter interface {
	Write(ctx context.Context, session GameSession) error
	Close()
}

type CassandraWriter struct {
	session *gocql.Session
}

func (c *CassandraWriter) Write(ctx context.Context, session GameSession) error {
	log.Printf("[Cassandra] Writing session: ID=%v, UserID=%s, Score=%d, GameMode=%s",
		session.SessionID, session.UserID, session.Score, session.GameMode)

	timer := prometheus.NewTimer(storageWriteDuration.WithLabelValues("cassandra"))
	defer timer.ObserveDuration()

	err := c.session.Query(
		`INSERT INTO game_system.game_sessions (session_id, user_id, score, game_mode, timestamp) VALUES (?, ?, ?, ?, ?)`,
		session.SessionID,
		session.UserID,
		session.Score,
		session.GameMode,
		session.Timestamp,
	).Exec()

	if err != nil {
		log.Printf("[Cassandra] Error writing session: %v", err)
		storageWriteErrors.WithLabelValues("cassandra").Inc()
		return err
	}

	log.Printf("[Cassandra] Successfully wrote session to database")
	return nil
}

func (c *CassandraWriter) Close() {
	c.session.Close()
}

type RedisWriter struct {
	client *redis.Client
}

func (r *RedisWriter) Write(ctx context.Context, session GameSession) error {
	leaderboardKey := "leaderboard:" + session.GameMode
	playerKey := "user:" + session.UserID

	log.Printf("[Redis] Updating leaderboard: Key=%s, Player=%s, Score=%d",
		leaderboardKey, playerKey, session.Score)

	timer := prometheus.NewTimer(storageWriteDuration.WithLabelValues("redis"))
	defer timer.ObserveDuration()

	err := r.client.ZIncrBy(ctx, leaderboardKey, float64(session.Score), playerKey).Err()
	if err != nil {
		log.Printf("[Redis] Error updating leaderboard: %v", err)
		storageWriteErrors.WithLabelValues("redis").Inc()
		return err
	}

	// Get updated score
	score, err := r.client.ZScore(ctx, leaderboardKey, playerKey).Result()
	if err != nil {
		log.Printf("[Redis] Error getting updated score: %v", err)
	} else {
		log.Printf("[Redis] Updated total score for %s: %.0f", playerKey, score)
	}

	return nil
}

func (r *RedisWriter) Close() {
	r.client.Close()
}

func setupCassandra() (*CassandraWriter, error) {
	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Port = 9042
	cluster.Keyspace = "game_system"
	cluster.Consistency = gocql.Quorum
	cluster.ProtoVersion = 4

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}

	return &CassandraWriter{session: session}, nil
}

func setupRedis() (*RedisWriter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	return &RedisWriter{client: client}, nil
}

func processMessages(ctx context.Context, writer StorageWriter, mode string) error {
	// Create a new reader with a specific partition
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{"localhost:9092"},
		Topic:       kafkaTopic,
		StartOffset: kafka.LastOffset,
	})
	defer r.Close()

	log.Println("starting message processor...")

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down message processor...")
			return nil
		default:
			msg, err := r.ReadMessage(ctx)
			if err != nil {
				if err.Error() == "EOF" || strings.Contains(err.Error(), "fetching message: EOF") {
					time.Sleep(time.Second)
					continue
				}
				log.Printf("error reading message: %v", err)
				messageProcessingErrors.Inc()
				time.Sleep(time.Second)
				continue
			}

			// Start timing message processing
			timer := prometheus.NewTimer(messageProcessingDuration)

			// Parse the message
			var kafkaMsg KafkaMessage
			log.Printf("message: %s", string(msg.Value))
			if err := json.Unmarshal(msg.Value, &kafkaMsg); err != nil {
				log.Printf("error unmarshaling message: %v", err)
				messageProcessingErrors.Inc()
				timer.ObserveDuration() 
				continue
			}

			// Increment game mode counter
			gameModeCounter.WithLabelValues(kafkaMsg.Session.GameMode).Inc()

			// Write the session to the database
			if err := writer.Write(ctx, kafkaMsg.Session); err != nil {
				log.Printf("error writing session: %v", err)
				messageProcessingErrors.Inc()
				timer.ObserveDuration() 
				continue
			}

			// Record successful processing
			messagesProcessed.Inc()
			timer.ObserveDuration()

			log.Printf("processed message from partition %d offset %d: score %d for user %s",
				msg.Partition, msg.Offset, kafkaMsg.Session.Score, kafkaMsg.Session.UserID)
		}
	}
}

func main() {

	mode := flag.String("mode", "", "storage mode (redis or cassandra)")
	flag.Parse()

	if *mode != "redis" && *mode != "cassandra" {
		log.Fatal("invalid mode, must be 'redis' or 'cassandra'")
	}

	var writer StorageWriter
	var err error

	if *mode == "redis" {

		go func() {
			http.Handle("/metrics", promhttp.Handler())
			log.Println("metrics endpoint running on :2112/metrics")
			log.Fatal(http.ListenAndServe(":2112", nil))
		}()
		writer, err = setupRedis()

	} else {

		go func() {
			http.Handle("/metrics", promhttp.Handler())
			log.Println("metrics endpoint running on :2113/metrics")
			log.Fatal(http.ListenAndServe(":2113", nil))
		}()
		writer, err = setupCassandra()
	}
	if err != nil {
		log.Fatalf("failed to setup %s: %v", *mode, err)
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the message processor
	if err := processMessages(ctx, writer, *mode); err != nil {
		log.Fatalf("failed to process messages: %v", err)
	}

	log.Printf("worker service started in %s mode", *mode)
}
