package totp_test

import (
	"strings"
	"testing"
	"time"

	"secure-auth-cli/internal/totp"

	pquernaTOTP "github.com/pquerna/otp/totp"
)

func TestGenerateSecretAndValidate(t *testing.T) {
	username := "testtotpuser"
	secret, otpauthURL, err := totp.GenerateSecret(username)
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}

	if secret == "" {
		t.Error("Expected non-empty secret")
	}

	if !strings.Contains(otpauthURL, "otpauth://totp/") || !strings.Contains(otpauthURL, "SecureAuthCLI") {
		t.Errorf("Unexpected otpauthURL format: %s", otpauthURL)
	}

	// Generate valid passcode for current time
	code, err := pquernaTOTP.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("Failed to generate test code: %v", err)
	}

	// Validate code
	if !totp.ValidateCode(secret, code) {
		t.Error("Expected valid TOTP code to pass validation")
	}

	// Validate invalid code
	if totp.ValidateCode(secret, "000000") && code != "000000" {
		t.Error("Expected invalid TOTP code to fail validation")
	}
}
