package main

import (
	"fmt"
	"os"

	"secure-auth-cli/internal/db"
	"secure-auth-cli/internal/shell"

	"github.com/ergochat/readline"
	"github.com/fatih/color"
)

func main() {
	// Configure distinct color formatters using fatih/color
	bannerTitle := color.New(color.FgCyan, color.Bold).SprintfFunc()
	bannerSub := color.New(color.FgHiBlue).SprintfFunc()
	statusColor := color.New(color.FgGreen).SprintfFunc()
	errorColor := color.New(color.FgRed).SprintfFunc()

	// Print welcome banner (app name + one-line description)
	fmt.Println(bannerTitle("=== Secure Auth CLI ==="))
	fmt.Println(bannerSub("Containerized CLI authentication system with optional 2FA and session management"))
	fmt.Println()

	// Initialize database connection and run schema migrations
	database, err := db.Open("")
	if err != nil {
		fmt.Println(errorColor("Database error: %v", err))
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		fmt.Println(errorColor("Database migration failed: %v", err))
		os.Exit(1)
	}

	fmt.Println(statusColor("Database ready"))
	fmt.Println()

	// Initialize readline instance with custom prompt and history support
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "auth> ",
		HistoryFile:     ".readline_history",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting interactive shell: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	// Start interactive shell REPL
	sh := shell.New(database, rl)
	if err := sh.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Shell error: %v\n", err)
		os.Exit(1)
	}
}
