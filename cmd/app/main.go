package main

import (
	"fmt" // Added import for fmt
	"log"
	"os"

	"github.com/user/go-argo-lite/internal/app"
	"github.com/user/go-argo-lite/internal/config"
)

func main() {
	// Set up basic logging
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	log.Println("Starting go-argo-lite application...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Error loading configuration: %v", err)
		// Use a more specific exit code or error handling as needed
		// For now, distinguish from general errors in app.Run()
		// For critical config errors, we might not want to os.Exit(1) directly
		// but rather ensure logs are flushed, etc.
		// However, for a simple app, this is often acceptable.
		// Consider structured logging for more complex scenarios.
		// log.Fatal() would print and then os.Exit(1)
		os.Stderr.WriteString(fmt.Sprintf("Error loading configuration: %v\n", err))
		os.Exit(2) // Specific exit code for config errors
	}

	log.Printf("Configuration loaded: %+v\n", cfg)

	// Create a new App instance
	application, err := app.NewApp(cfg)
	if err != nil {
		log.Printf("Error creating application: %v", err)
		os.Stderr.WriteString(fmt.Sprintf("Error creating application: %v\n", err))
		os.Exit(3) // Specific exit code for app creation errors
	}

	log.Println("Application instance created. Starting Run()...")
	// Run the application
	if err := application.Run(); err != nil {
		log.Printf("Application run failed: %v", err)
		os.Stderr.WriteString(fmt.Sprintf("Application run failed: %v\n", err))
		os.Exit(1) // General application error
	}

	log.Println("Application shut down successfully.")
	os.Exit(0)
}
