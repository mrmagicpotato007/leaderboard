package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"userservice/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	db                 *sql.DB
	jwtSecret          = []byte("secret-test")
)

func isDuplicateKeyError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unique constraint")
}

func setUpApplication() {
	db = setupDatabase()
}

func main() {

	setUpApplication()
	defer db.Close()

	r := mux.NewRouter()
	r.HandleFunc("/v1/signup", signupHandler).Methods("POST")
	r.HandleFunc("/v1/login", loginHandler).Methods("POST")

	//inter service api no auth needed
	r.HandleFunc("/v1/UserInfo", getUserInfo).Methods("POST")

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	log.Printf("user service starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func setupDatabase() *sql.DB {
	connStr := "postgres://postgres:mysecretpassword@localhost:5432/postgres?sslmode=disable"

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return db
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if !user.ValidateUserName() {
		http.Error(w, "Username must be between 3 and 50 characters", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec(
		"INSERT INTO users (username, password) VALUES ($1, $2)",
		user.Username,
		string(hashedPassword),
	)

	if err != nil {
		if isDuplicateKeyError(err) {
			http.Error(w, "Username is already taken", http.StatusConflict)
			return
		}
		log.Printf("Error creating user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"message": "User created successfully"}`))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("login handler called")

	var credentials models.User
	if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	var storedUser models.User
	err := db.QueryRow(
		"SELECT id, username, password FROM users WHERE username = $1",
		credentials.Username,
	).Scan(&storedUser.ID, &storedUser.Username, &storedUser.Password)

	if err == sql.ErrNoRows {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	} else if err != nil {
		log.Printf("Database error during login: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(storedUser.Password),
		[]byte(credentials.Password),
	); err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  storedUser.ID,
		"username": storedUser.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"token": "` + tokenString + `"}`))
}

func getUserInfo(w http.ResponseWriter, r *http.Request) {
	var userIds []string
	if err := json.NewDecoder(r.Body).Decode(&userIds); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	log.Println("userIds", userIds)

	var users []models.User
	for _, userId := range userIds {
		var user models.User
		err := db.QueryRow(
			"SELECT id, username, join_date FROM users WHERE id = $1",
			userId,
		).Scan(&user.ID, &user.Username, &user.JoinDate)

		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Database error during user lookup: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		users = append(users, user)
	}

	log.Println("users", users)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}
