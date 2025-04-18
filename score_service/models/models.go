package models

import (
	"errors"
	"regexp"
	"time"

	"github.com/gocql/gocql"
)

const (
	MaxScore        = 500
	MinScore        = 0
	ScoreExpiration = 5 * time.Minute
)

type GameSession struct {
	SessionID gocql.UUID `json:"session_id"`
	UserID    string     `json:"user_id"`
	Score     int        `json:"score"`
	GameMode  string     `json:"game_mode"`
	Timestamp time.Time  `json:"timestamp"`
}

func (s *GameSession) ValidateSession() error {
	if s.Score < MinScore || s.Score > MaxScore {
		return errors.New("invalid score value")
	}

	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(s.GameMode) {
		return errors.New("invalid game mode format")
	}

	return nil
}
