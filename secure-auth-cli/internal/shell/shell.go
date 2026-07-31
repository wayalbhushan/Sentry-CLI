// Package shell handles interactive CLI prompt execution, history, and command dispatching.
package shell

import (
	"database/sql"
	"fmt"
	"io"
	"strings"

	"secure-auth-cli/internal/auth"
	"secure-auth-cli/internal/session"
	"secure-auth-cli/internal/totp"

	"github.com/ergochat/readline"
	"github.com/fatih/color"
)

// Shell encapsulates the state and dependencies of the interactive CLI session.
type Shell struct {
	db           *sql.DB
	rl           *readline.Instance
	currentUser  *auth.User
	sessionToken string

	promptColor   func(a ...interface{}) string
	statusColor   func(format string, a ...interface{}) string
	errorColor    func(format string, a ...interface{}) string
	infoColor     func(format string, a ...interface{}) string
	farewellColor func(format string, a ...interface{}) string
}

// New creates a new instance of Shell with configured output formatters.
func New(db *sql.DB, rl *readline.Instance) *Shell {
	return &Shell{
		db: db,
		rl: rl,

		// Darker rich blue for prompt visibility contrasting against white text
		promptColor:   color.New(color.FgBlue, color.Bold).SprintFunc(),
		statusColor:   color.New(color.FgGreen, color.Bold).SprintfFunc(),
		errorColor:    color.New(color.FgRed, color.Bold).SprintfFunc(),
		infoColor:     color.New(color.FgYellow, color.Bold).SprintfFunc(),
		farewellColor: color.New(color.FgGreen, color.Bold).SprintfFunc(),
	}
}

// Run executes the main Read-Eval-Print Loop (REPL).
func (s *Shell) Run() error {
	s.updatePrompt()

	for {
		line, err := s.rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				// Handle Ctrl+C on empty input without terminating
				if len(line) == 0 {
					continue
				}
			} else if err == io.EOF {
				// Handle Ctrl+D gracefully
				fmt.Println(s.farewellColor("Goodbye!"))
				break
			}
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		cmd := strings.ToLower(parts[0])

		if cmd == "exit" {
			fmt.Println(s.farewellColor("Goodbye!"))
			break
		}

		s.dispatchCommand(cmd, parts[1:])
	}

	return nil
}

// updatePrompt refreshes the REPL prompt string based on session authentication state.
func (s *Shell) updatePrompt() {
	if s.sessionToken != "" && s.currentUser != nil {
		s.rl.SetPrompt(fmt.Sprintf("%s@auth> ", s.currentUser.Username))
	} else {
		s.rl.SetPrompt("auth> ")
	}
}

// checkSession validates the active session token before executing post-login commands.
// If invalid or expired, clears session state, prints red error message, and resets prompt.
func (s *Shell) checkSession() bool {
	if s.sessionToken == "" {
		return false
	}

	_, err := session.ValidateSession(s.db, s.sessionToken)
	if err != nil {
		fmt.Println(s.errorColor("Error: %v", err))
		s.sessionToken = ""
		s.currentUser = nil
		s.updatePrompt()
		return false
	}

	return true
}

// readPassword prompts for a password with a visible prompt and masked terminal echo.
// Supports both interactive TTY raw mode and piped stdin fallbacks.
func (s *Shell) readPassword(promptLabel string) (string, error) {
	if promptLabel != "" {
		fmt.Print(s.promptColor(promptLabel))
	}

	passBytes, err := s.rl.ReadPassword("")
	if err != nil && err != io.EOF {
		return "", err
	}

	password := strings.TrimSpace(string(passBytes))
	if password == "" {
		// Fallback to reading next line if ReadPassword returns empty in non-TTY pipe environments
		line, err := s.rl.Readline()
		if err != nil {
			return "", err
		}
		password = strings.TrimSpace(line)
	}

	return password, nil
}

// readInput prompts for non-sensitive line input with prompt styling.
func (s *Shell) readInput(promptLabel string) (string, error) {
	if promptLabel != "" {
		fmt.Print(s.promptColor(promptLabel))
	}

	line, err := s.rl.Readline()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// dispatchCommand routes user input commands to their respective handlers.
func (s *Shell) dispatchCommand(cmd string, args []string) {
	switch cmd {
	case "help":
		s.handleHelp()

	case "register":
		if s.sessionToken != "" {
			if !s.checkSession() {
				s.handleRegister(args)
				return
			}
			fmt.Println(s.errorColor("Error: You are already logged in. Please logout first to register a new account."))
			return
		}
		s.handleRegister(args)

	case "login":
		if s.sessionToken != "" {
			if !s.checkSession() {
				s.handleLogin(args)
				return
			}
			fmt.Println(s.errorColor("Error: You are already logged in as %s.", s.currentUser.Username))
			return
		}
		s.handleLogin(args)

	case "whoami":
		if !s.checkSession() {
			if s.sessionToken == "" {
				fmt.Println(s.errorColor("Error: unrecognized command: whoami (please login first)"))
			}
			return
		}
		s.handleWhoAmI()

	case "logout":
		if !s.checkSession() {
			if s.sessionToken == "" {
				fmt.Println(s.errorColor("Error: unrecognized command: logout (no active session)"))
			}
			return
		}
		s.handleLogout()

	case "enable-2fa":
		if !s.checkSession() {
			if s.sessionToken == "" {
				fmt.Println(s.errorColor("Error: unrecognized command: enable-2fa (please login first)"))
			}
			return
		}
		s.handleEnable2FA()

	case "disable-2fa":
		if !s.checkSession() {
			if s.sessionToken == "" {
				fmt.Println(s.errorColor("Error: unrecognized command: disable-2fa (please login first)"))
			}
			return
		}
		s.handleDisable2FA()

	default:
		fmt.Println(s.errorColor("Error: unrecognized command: %s", cmd))
	}
}

// handleHelp displays commands appropriate for current session state.
func (s *Shell) handleHelp() {
	isLoggedIn := s.sessionToken != "" && s.checkSession()
	fmt.Println(s.infoColor("Available commands:"))
	if !isLoggedIn {
		fmt.Println("  register     - Register a new user account")
		fmt.Println("  login        - Login with username and password (+ 2FA if enabled)")
		fmt.Println("  help         - Display this help menu")
		fmt.Println("  exit         - Quit the application")
	} else {
		fmt.Println("  whoami       - Display active user session details")
		fmt.Println("  enable-2fa   - Enable TOTP 2FA multi-factor authentication")
		fmt.Println("  disable-2fa  - Disable TOTP 2FA multi-factor authentication")
		fmt.Println("  logout       - End current session")
		fmt.Println("  help         - Display this help menu")
		fmt.Println("  exit         - Quit the application")
	}
}

// handleRegister prompts for credentials and registers a new user.
func (s *Shell) handleRegister(args []string) {
	var username string
	if len(args) > 0 {
		username = args[0]
	} else {
		line, err := s.readInput("Enter username: ")
		s.updatePrompt()
		if err != nil {
			return
		}
		username = line
	}

	if username == "" {
		fmt.Println(s.errorColor("Error: Username cannot be empty."))
		return
	}

	// Check if username exists immediately before asking for password
	exists, err := auth.UserExists(s.db, username)
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to verify username: %v", err))
		return
	}
	if exists {
		fmt.Println(s.errorColor("Error: user '%s' already exists. Try logging in using 'login %s'", username, username))
		return
	}

	password, err := s.readPassword("Enter password: ")
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to read password: %v", err))
		return
	}

	if err := auth.Register(s.db, username, password); err != nil {
		fmt.Println(s.errorColor("Error: %v", err))
		return
	}

	fmt.Println(s.statusColor("Registration successful! You can now log in using 'login'."))
}

// handleLogin prompts for credentials, verifies bcrypt hash & TOTP, creates DB session, and transitions state.
func (s *Shell) handleLogin(args []string) {
	var username string
	if len(args) > 0 {
		username = args[0]
	} else {
		line, err := s.readInput("Enter username: ")
		s.updatePrompt()
		if err != nil {
			return
		}
		username = line
	}

	if username == "" {
		fmt.Println(s.errorColor("Error: Username cannot be empty."))
		return
	}

	password, err := s.readPassword("Enter password: ")
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to read password: %v", err))
		return
	}

	user, err := auth.Login(s.db, username, password)
	if err != nil {
		fmt.Println(s.errorColor("Error: %v", err))
		return
	}

	// If user has TOTP 2FA enabled, require TOTP verification before creating session
	if user.TOTPEnabled {
		totpCode, err := s.readInput("Enter your 6-digit authenticator code: ")
		if err != nil {
			fmt.Println(s.errorColor("Error: Failed to read TOTP code: %v", err))
			return
		}

		if !totp.ValidateCode(user.TOTPSecret.String, totpCode) {
			// Record failed authentication attempt for lockout threshold tracking
			_ = auth.RecordFailedAttempt(s.db, user.ID)
			fmt.Println(s.errorColor("Error: Invalid 2FA verification code."))
			return
		}

		// Finalize login (reset failed attempts & set last login time) after successful TOTP
		if err := auth.CompleteLoginFinalize(s.db, user); err != nil {
			fmt.Println(s.errorColor("Error: Failed to finalize login: %v", err))
			return
		}
	}

	sess, err := session.CreateSession(s.db, user.ID)
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to create session: %v", err))
		return
	}

	s.sessionToken = sess.Token
	s.currentUser = user
	s.updatePrompt()

	fmt.Println(s.statusColor("Logged in as %s", user.Username))
}

// handleEnable2FA generates TOTP secret, renders terminal QR code, and confirms passcode before persisting.
func (s *Shell) handleEnable2FA() {
	if s.currentUser.TOTPEnabled {
		fmt.Println(s.infoColor("2FA is already enabled for this account."))
		return
	}

	secret, otpauthURL, err := totp.GenerateSecret(s.currentUser.Username)
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to generate 2FA secret: %v", err))
		return
	}

	fmt.Println(s.infoColor("\nScan the QR code below with your authenticator app (Google Authenticator / Authy):"))
	totp.RenderQRCode(otpauthURL)
	fmt.Printf("Secret Key (manual entry): %s\n\n", secret)

	code, err := s.readInput("Enter the 6-digit code from your authenticator app to confirm: ")
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to read verification code: %v", err))
		return
	}

	if !totp.ValidateCode(secret, code) {
		fmt.Println(s.errorColor("Error: Invalid 2FA verification code. 2FA was not enabled."))
		return
	}

	if err := auth.Enable2FA(s.db, s.currentUser.ID, secret); err != nil {
		fmt.Println(s.errorColor("Error: Failed to enable 2FA: %v", err))
		return
	}

	s.currentUser.TOTPSecret = sql.NullString{String: secret, Valid: true}
	s.currentUser.TOTPEnabled = true

	fmt.Println(s.statusColor("2FA enabled successfully!"))
}

// handleDisable2FA verifies user password before disabling 2FA and clearing secret from database.
func (s *Shell) handleDisable2FA() {
	if !s.currentUser.TOTPEnabled {
		fmt.Println(s.infoColor("2FA is not currently enabled for this account."))
		return
	}

	password, err := s.readPassword("Enter your current password to confirm disabling 2FA: ")
	if err != nil {
		fmt.Println(s.errorColor("Error: Failed to read password: %v", err))
		return
	}

	if !auth.VerifyPassword(s.currentUser.PasswordHash, password) {
		fmt.Println(s.errorColor("Error: Incorrect password. 2FA disabling cancelled."))
		return
	}

	if err := auth.Disable2FA(s.db, s.currentUser.ID); err != nil {
		fmt.Println(s.errorColor("Error: Failed to disable 2FA: %v", err))
		return
	}

	s.currentUser.TOTPSecret = sql.NullString{Valid: false}
	s.currentUser.TOTPEnabled = false

	fmt.Println(s.statusColor("2FA disabled successfully."))
}

// handleWhoAmI displays details of the currently logged-in user.
func (s *Shell) handleWhoAmI() {
	if s.currentUser == nil {
		return
	}
	fmt.Printf("Logged in as: %s\n", s.currentUser.Username)
	fmt.Printf("Registered on: %s\n", s.currentUser.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	if s.currentUser.LastLoginAt.Valid {
		fmt.Printf("Last login: %s\n", s.currentUser.LastLoginAt.Time.Local().Format("2006-01-02 15:04:05"))
	}
	if s.currentUser.TOTPEnabled {
		fmt.Printf("MFA Status: %s\n", color.New(color.FgGreen, color.Bold).Sprint("Enabled"))
	} else {
		fmt.Printf("MFA Status: %s\n", color.New(color.FgYellow, color.Bold).Sprint("Disabled"))
	}
}

// handleLogout ends the active user session in database and resets shell state.
func (s *Shell) handleLogout() {
	if s.sessionToken != "" {
		_ = session.Logout(s.db, s.sessionToken)
	}
	s.sessionToken = ""
	s.currentUser = nil
	s.updatePrompt()
	fmt.Println(s.statusColor("Logged out successfully."))
}
