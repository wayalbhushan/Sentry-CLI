// Package shell handles interactive CLI prompt execution, history, and command dispatching.
package shell

import (
	"database/sql"
	"fmt"
	"io"
	"strings"

	"secure-auth-cli/internal/auth"

	"github.com/ergochat/readline"
	"github.com/fatih/color"
)

// Shell encapsulates the state and dependencies of the interactive CLI session.
type Shell struct {
	db          *sql.DB
	rl          *readline.Instance
	currentUser *auth.User
	isLoggedIn  bool

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

// updatePrompt refreshes the REPL prompt string based on authentication state.
func (s *Shell) updatePrompt() {
	if s.isLoggedIn && s.currentUser != nil {
		s.rl.SetPrompt(fmt.Sprintf("%s@auth> ", s.currentUser.Username))
	} else {
		s.rl.SetPrompt("auth> ")
	}
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

// dispatchCommand routes user input commands to their respective handlers.
func (s *Shell) dispatchCommand(cmd string, args []string) {
	switch cmd {
	case "help":
		s.handleHelp()

	case "register":
		if s.isLoggedIn {
			fmt.Println(s.errorColor("Error: You are already logged in. Please logout first to register a new account."))
			return
		}
		s.handleRegister(args)

	case "login":
		if s.isLoggedIn {
			fmt.Println(s.errorColor("Error: You are already logged in as %s.", s.currentUser.Username))
			return
		}
		s.handleLogin(args)

	case "whoami":
		if !s.isLoggedIn {
			fmt.Println(s.errorColor("Error: unrecognized command: whoami (please login first)"))
			return
		}
		s.handleWhoAmI()

	case "logout":
		if !s.isLoggedIn {
			fmt.Println(s.errorColor("Error: unrecognized command: logout (no active session)"))
			return
		}
		s.handleLogout()

	case "enable-2fa", "disable-2fa":
		if !s.isLoggedIn {
			fmt.Println(s.errorColor("Error: unrecognized command: %s (please login first)", cmd))
			return
		}
		fmt.Println(s.infoColor("[%s] TOTP 2FA will be implemented in Phase 4.", cmd))

	default:
		fmt.Println(s.errorColor("Error: unrecognized command: %s", cmd))
	}
}

// handleHelp displays commands appropriate for current session state.
func (s *Shell) handleHelp() {
	fmt.Println(s.infoColor("Available commands:"))
	if !s.isLoggedIn {
		fmt.Println("  register     - Register a new user account")
		fmt.Println("  login        - Login with username and password")
		fmt.Println("  help         - Display this help menu")
		fmt.Println("  exit         - Quit the application")
	} else {
		fmt.Println("  whoami       - Display active user session details")
		fmt.Println("  enable-2fa   - Enable TOTP 2FA (Phase 4)")
		fmt.Println("  disable-2fa  - Disable TOTP 2FA (Phase 4)")
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
		fmt.Print(s.promptColor("Enter username: "))
		line, err := s.rl.Readline()
		s.updatePrompt()
		if err != nil {
			return
		}
		username = strings.TrimSpace(line)
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

// handleLogin prompts for credentials, verifies bcrypt hash, and transitions to post-login state.
func (s *Shell) handleLogin(args []string) {
	var username string
	if len(args) > 0 {
		username = args[0]
	} else {
		fmt.Print(s.promptColor("Enter username: "))
		line, err := s.rl.Readline()
		s.updatePrompt()
		if err != nil {
			return
		}
		username = strings.TrimSpace(line)
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

	s.currentUser = user
	s.isLoggedIn = true
	s.updatePrompt()

	fmt.Println(s.statusColor("Logged in as %s", user.Username))
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
}

// handleLogout ends the active user session.
func (s *Shell) handleLogout() {
	s.currentUser = nil
	s.isLoggedIn = false
	s.updatePrompt()
	fmt.Println(s.statusColor("Logged out successfully."))
}
