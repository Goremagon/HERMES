package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	PasswordCost     = 10
	SessionTokenSize = 32
)

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Session struct {
	Token     string
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hashed), nil
}

func ComparePassword(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("compare password: %w", err)
	}
	return nil
}

func GenerateSessionToken() (string, error) {
	buf := make([]byte, SessionTokenSize)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func GetSession(ctx context.Context, db *sql.DB, token string) (Session, error) {
	if token == "" {
		return Session{}, fmt.Errorf("empty session token")
	}

	var (
		session       Session
		expiresAtText string
	)
	err := db.QueryRowContext(
		ctx,
		`SELECT sessions.token, sessions.user_id, users.username, sessions.expires_at
		 FROM sessions
		 JOIN users ON users.id = sessions.user_id
		 WHERE sessions.token = ?`,
		token,
	).Scan(&session.Token, &session.UserID, &session.Username, &expiresAtText)
	if err != nil {
		return Session{}, fmt.Errorf("fetch session: %w", err)
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtText)
	if err != nil {
		return Session{}, fmt.Errorf("parse session expiry: %w", err)
	}
	session.ExpiresAt = expiresAt

	if time.Now().UTC().After(session.ExpiresAt) {
		_, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
		return Session{}, fmt.Errorf("session expired")
	}

	return session, nil
}
