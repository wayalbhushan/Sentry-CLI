package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"secure-auth-cli/internal/db"

	"github.com/ergochat/readline"
	"github.com/fatih/color"
)

func main() {
	// Configure distinct color formatters using fatih/color
	bannerTitle := color.New(color.FgCyan, color.Bold).SprintfFunc()
	bannerSub := color.New(color.FgHiBlue).SprintfFunc()
	statusColor := color.New(color.FgGreen).SprintfFunc()
	errorColor := color.New(color.FgRed).SprintfFunc()
	farewellColor := color.New(color.FgGreen).SprintfFunc()

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

	// Read-Eval-Print Loop (REPL)
	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				// Gracefully handle Ctrl+C without panicking
				if len(line) == 0 {
					continue
				}
			} else if err == io.EOF {
				// Gracefully handle Ctrl+D: farewell and exit
				fmt.Println(farewellColor("Goodbye!"))
				break
			}
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" {
			fmt.Println(farewellColor("Goodbye!"))
			break
		}

		// Output unrecognized command message in red
		fmt.Println(errorColor("unrecognized command: %s", input))
	}
}
