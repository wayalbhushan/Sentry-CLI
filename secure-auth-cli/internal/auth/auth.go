// Package auth owns registration, login, password hashing, and lockout state.
package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Default authentication configuration constants
const (
	DefaultLockoutThreshold       = 5
	DefaultLockoutDurationMinutes = 15
	BcryptCost                    = 10
)

// User represents a user record from the database.
type User struct {
	ID                  int64
	Username            string
	PasswordHash        string
	TOTPSecret          sql.NullString
	TOTPEnabled         bool
	CreatedAt           time.Time
	LastLoginAt         sql.NullTime
	FailedLoginAttempts int
	LockedUntil         sql.NullTime
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

// Register creates a new user account with a bcrypt hashed password.
func Register(db *sql.DB, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username cannot be empty")
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	// Check if username already exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check username existence: %w", err)
	}
	if exists {
		return fmt.Errorf("user '%s' already exists. Try logging in using 'login %s'", username, username)
	}

	// Hash password using bcrypt with cost 10
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user into database
	insertSQL := `
	INSERT INTO users (username, password_hash, totp_enabled, failed_login_attempts)
	VALUES (?, ?, 0, 0);`
	if _, err := db.Exec(insertSQL, username, string(hash)); err != nil {
		return fmt.Errorf("failed to insert new user record: %w", err)
	}

	return nil
}

// Login authenticates a user by username and password.
// Enforces account lockout threshold and duration upon multiple failed login attempts.
func Login(db *sql.DB, username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("invalid username or password. Please check your credentials and try again.")
	}

	lockoutThreshold := getEnvInt("LOCKOUT_THRESHOLD", DefaultLockoutThreshold)
	lockoutDurationMinutes := getEnvInt("LOCKOUT_DURATION_MINUTES", DefaultLockoutDurationMinutes)

	// Fetch user details from database
	var user User
	query := `
	SELECT id, username, password_hash, totp_secret, totp_enabled, created_at, last_login_at, failed_login_attempts, locked_until
	FROM users
	WHERE username = ?;`

	err := db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.TOTPSecret,
		&user.TOTPEnabled,
		&user.CreatedAt,
		&user.LastLoginAt,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// Generic error to prevent username enumeration while providing user feedback
		return nil, errors.New("invalid username or password. Please check your credentials and try again.")
	} else if err != nil {
		return nil, fmt.Errorf("database query failed during login: %w", err)
	}

	now := time.Now().UTC()

	// Check if account is currently locked
	if user.LockedUntil.Valid && user.LockedUntil.Time.After(now) {
		formattedTime := user.LockedUntil.Time.Local().Format("15:04:05 MST")
		return nil, fmt.Errorf("account is locked due to multiple failed login attempts. Try again after %s", formattedTime)
	}

	// Verify password against stored bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Increment failed login attempts
		newAttempts := user.FailedLoginAttempts + 1
		if newAttempts >= lockoutThreshold {
			lockedUntil := now.Add(time.Duration(lockoutDurationMinutes) * time.Minute)
			updateSQL := `UPDATE users SET failed_login_attempts = 0, locked_until = ? WHERE id = ?;`
			if _, execErr := db.Exec(updateSQL, lockedUntil, user.ID); execErr != nil {
				return nil, fmt.Errorf("failed to set account lockout: %w", execErr)
			}
		} else {
			updateSQL := `UPDATE users SET failed_login_attempts = ? WHERE id = ?;`
			if _, execErr := db.Exec(updateSQL, newAttempts, user.ID); execErr != nil {
				return nil, fmt.Errorf("failed to record failed login attempt: %w", execErr)
			}
		}

		return nil, errors.New("invalid username or password. Please check your credentials and try again.")
	}

	// On successful password match: reset failed login attempts, clear lockout, update last login time
	updateSuccessSQL := `UPDATE users SET failed_login_attempts = 0, locked_until = NULL, last_login_at = ? WHERE id = ?;`
	if _, execErr := db.Exec(updateSuccessSQL, now, user.ID); execErr != nil {
		return nil, fmt.Errorf("failed to update login success metadata: %w", execErr)
	}

	user.FailedLoginAttempts = 0
	user.LockedUntil = sql.NullTime{Valid: false}
	user.LastLoginAt = sql.NullTime{Time: now, Valid: true}

	return &user, nil
}
