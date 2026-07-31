// Package totp handles time-based one-time password generation, QR code rendering, and MFA verification.
package totp

import (
	"fmt"
	"os"
	"strings"

	"github.com/mdp/qrterminal/v3"
	"github.com/pquerna/otp/totp"
)

const IssuerName = "SecureAuthCLI"

// GenerateSecret creates a new TOTP key for the specified username.
// Returns the base32 secret and the full otpauth:// provisioning URI.
func GenerateSecret(username string) (secret string, otpauthURL string, err error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", fmt.Errorf("username cannot be empty")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      IssuerName,
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// ValidateCode verifies a 6-digit passcode against the base32 TOTP secret.
// Utilizes RFC 6238 standard 30-second time steps with clock-skew tolerance.
func ValidateCode(secret, code string) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	return totp.Validate(code, secret)
}

// RenderQRCode prints a compact ANSI QR code to stdout for authenticator app scanning.
func RenderQRCode(otpauthURL string) {
	config := qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     os.Stdout,
		HalfBlocks: true,
		QuietZone:  1,
	}
	qrterminal.GenerateWithConfig(otpauthURL, config)
}
