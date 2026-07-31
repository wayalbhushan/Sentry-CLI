package session_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"secure-auth-cli/internal/db"
	"secure-auth-cli/internal/session"
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

	// Insert dummy user record for foreign key reference
	_, err = database.Exec("INSERT INTO users (id, username, password_hash) VALUES (1, 'sessuser', 'hash')")
	if err != nil {
		database.Close()
		t.Fatalf("Failed to insert dummy test user: %v", err)
	}

	return database
}

func TestCreateAndValidateSession(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	sess, err := session.CreateSession(database, 1)
	if err != nil {
		t.Fatalf("CreateSession failed unexpectedly: %v", err)
	}

	if len(sess.Token) != 64 { // 32 bytes hex encoded = 64 characters
		t.Errorf("Expected 64 char hex token, got length %d (%s)", len(sess.Token), sess.Token)
	}

	// Validate valid session
	validated, err := session.ValidateSession(database, sess.Token)
	if err != nil {
		t.Fatalf("ValidateSession failed for valid session: %v", err)
	}
	if validated.UserID != 1 {
		t.Errorf("Expected UserID 1, got %d", validated.UserID)
	}
}

func TestSessionExpiration(t *testing.T) {
	os.Setenv("SESSION_TIMEOUT_MINUTES", "1")
	defer os.Unsetenv("SESSION_TIMEOUT_MINUTES")

	database := setupTestDB(t)
	defer database.Close()

	sess, err := session.CreateSession(database, 1)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Manually expire session in DB
	pastTime := time.Now().UTC().Add(-2 * time.Minute)
	_, err = database.Exec("UPDATE sessions SET expires_at = ? WHERE token = ?", pastTime, sess.Token)
	if err != nil {
		t.Fatalf("Failed to update expires_at in DB: %v", err)
	}

	// Validation should fail with clear session expired error
	_, err = session.ValidateSession(database, sess.Token)
	if err == nil {
		t.Error("Expected error for expired session, got nil")
	}

	// Confirm session row was cleaned up lazily
	var count int
	_ = database.QueryRow("SELECT COUNT(1) FROM sessions WHERE token = ?", sess.Token).Scan(&count)
	if count != 0 {
		t.Errorf("Expected expired session row to be deleted, found %d rows", count)
	}
}

func TestLogout(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	sess, err := session.CreateSession(database, 1)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if err := session.Logout(database, sess.Token); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Validation after logout should fail
	_, err = session.ValidateSession(database, sess.Token)
	if err == nil {
		t.Error("Expected error after logout, got nil")
	}

	// Idempotent test
	if err := session.Logout(database, "nonexistent"); err != nil {
		t.Errorf("Logout for nonexistent token returned error: %v", err)
	}
}
