package models

import "time"

type User struct {
	ID       int       `json:"id"`
	Username string    `json:"username"`
	Password string    `json:"password,omitempty"`
	JoinDate time.Time `json:"join_date"`
}

func (u *User) ValidateUserName() bool {
	if len(u.Username) < 3 || len(u.Username) > 50 {
		return false
	}
	return true
}

type LoginResponse struct {
	Token string `json:"token"`
}
