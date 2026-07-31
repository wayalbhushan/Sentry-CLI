package auth_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"secure-auth-cli/internal/auth"
	"secure-auth-cli/internal/db"

	"golang.org/x/crypto/bcrypt"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open test in-memory database: %v", err)
	}

	if err := db.Migrate(database); err != nil {
		database.Close()
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return database
}

func TestRegister(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	// Test successful registration
	err := auth.Register(database, "testuser", "securepass123")
	if err != nil {
		t.Fatalf("Register failed unexpectedly: %v", err)
	}

	// Verify row in database and bcrypt hash
	var passwordHash string
	err = database.QueryRow("SELECT password_hash FROM users WHERE username = ?", "testuser").Scan(&passwordHash)
	if err != nil {
		t.Fatalf("Failed to query inserted user: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("securepass123")); err != nil {
		t.Errorf("Password hash does not match original password: %v", err)
	}

	// Test duplicate registration error
	err = auth.Register(database, "testuser", "anotherpass")
	if err == nil {
		t.Error("Expected error when registering duplicate username, got nil")
	}

	// Test empty credentials error
	if err := auth.Register(database, "", "pass"); err == nil {
		t.Error("Expected error for empty username")
	}
	if err := auth.Register(database, "user2", ""); err == nil {
		t.Error("Expected error for empty password")
	}
}

func TestLoginAndLockout(t *testing.T) {
	os.Setenv("LOCKOUT_THRESHOLD", "3")
	os.Setenv("LOCKOUT_DURATION_MINUTES", "1")
	defer func() {
		os.Unsetenv("LOCKOUT_THRESHOLD")
		os.Unsetenv("LOCKOUT_DURATION_MINUTES")
	}()

	database := setupTestDB(t)
	defer database.Close()

	username := "lockoutuser"
	password := "correctpassword"

	if err := auth.Register(database, username, password); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Test successful login
	user, err := auth.Login(database, username, password)
	if err != nil {
		t.Fatalf("Login failed with correct credentials: %v", err)
	}
	if user.Username != username {
		t.Errorf("Expected username %s, got %s", username, user.Username)
	}

	// Test failed login attempts up to threshold (3)
	for i := 1; i <= 2; i++ {
		_, err := auth.Login(database, username, "wrongpassword")
		if err == nil || err.Error() != "invalid username or password" {
			t.Errorf("Attempt %d: expected generic 'invalid username or password', got: %v", i, err)
		}
	}

	// 3rd failed attempt triggers lockout
	_, err = auth.Login(database, username, "wrongpassword")
	if err == nil {
		t.Error("3rd attempt: expected error, got nil")
	}

	// 4th attempt while locked should return lockout error message
	_, err = auth.Login(database, username, password)
	if err == nil {
		t.Error("Expected lockout error even with correct password while locked")
	}

	// Fast-forward lockout in DB for testing recovery
	expiredTime := time.Now().UTC().Add(-2 * time.Minute)
	_, err = database.Exec("UPDATE users SET locked_until = ? WHERE username = ?", expiredTime, username)
	if err != nil {
		t.Fatalf("Failed to update locked_until for test: %v", err)
	}

	// Login after lockout expires should succeed
	recoveredUser, err := auth.Login(database, username, password)
	if err != nil {
		t.Fatalf("Expected successful login after lockout expired, got: %v", err)
	}
	if recoveredUser.Username != username {
		t.Errorf("Expected username %s, got %s", username, recoveredUser.Username)
	}
}
