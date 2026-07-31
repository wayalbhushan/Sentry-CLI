// Package session manages user session state, tokens, and configurable expiration timeouts.
package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultSessionTimeoutMinutes = 15
	TokenByteLength              = 32
)

// Session represents an active user session record.
type Session struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// getEnvInt retrieves an integer environment variable or returns the default value.
func getEnvInt(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

// CreateSession generates a secure 32-byte hex token and inserts a new session record into the database.
func CreateSession(db *sql.DB, userID int64) (*Session, error) {
	if db == nil {
		return nil, errors.New("database connection cannot be nil")
	}

	timeoutMinutes := getEnvInt("SESSION_TIMEOUT_MINUTES", DefaultSessionTimeoutMinutes)

	// Generate 32 bytes of cryptographically secure random data
	tokenBytes := make([]byte, TokenByteLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure session token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(timeoutMinutes) * time.Minute)

	insertSQL := `
	INSERT INTO sessions (user_id, token, expires_at, created_at)
	VALUES (?, ?, ?, ?);`

	res, err := db.Exec(insertSQL, userID, token, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("failed to store session in database: %w", err)
	}

	sessionID, err := res.LastInsertId()
	if err != nil {
		sessionID = 0
	}

	return &Session{
		ID:        sessionID,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

// ValidateSession verifies if a session token exists and has not expired.
func ValidateSession(db *sql.DB, token string) (*Session, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("session expired or invalid, please log in again")
	}

	var sess Session
	query := `
	SELECT id, user_id, token, expires_at, created_at
	FROM sessions
	WHERE token = ?;`

	err := db.QueryRow(query, token).Scan(
		&sess.ID,
		&sess.UserID,
		&sess.Token,
		&sess.ExpiresAt,
		&sess.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("session expired or invalid, please log in again")
	} else if err != nil {
		return nil, fmt.Errorf("database query failed during session validation: %w", err)
	}

	now := time.Now().UTC()
	if !sess.ExpiresAt.After(now) {
		// Clean up expired session lazily
		_ = Logout(db, token)
		return nil, errors.New("session expired or invalid, please log in again")
	}

	return &sess, nil
}

// Logout deletes the session record matching the provided token.
// Idempotent: deleting a nonexistent token does not return an error.
func Logout(db *sql.DB, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	deleteSQL := `DELETE FROM sessions WHERE token = ?;`
	if _, err := db.Exec(deleteSQL, token); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}
