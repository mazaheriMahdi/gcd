package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	// "github.com/user/go-argo-lite/internal/config" // No longer using config.LoadConfig()
	"github.com/user/go-argo-lite/internal/server"
	"github.com/user/go-argo-lite/internal/storage"
	"github.com/user/go-argo-lite/internal/synctarget" // For GetEncryptionKey (if we make it public) or key consts
	"github.com/user/go-argo-lite/internal/worker"
)

const (
	// DefaultSyncTargetsFile is the default path for the sync targets JSON file.
	// Duplicated from internal/synctarget/file_provider.go and internal/storage/encrypted_file_storage.go
	// Consider moving to a shared constants package if this becomes unwieldy.
	DefaultSyncTargetsFile = "synctargets.json.enc"
	// EnvEncryptionKey is the environment variable for the encryption key.
	// Duplicated from internal/synctarget/file_provider.go and internal/storage/encrypted_file_storage.go
	EnvEncryptionKey = "GO_ARGO_LITE_ENCRYPTION_KEY"
)

func main() {
	// Set up basic logging
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	log.Println("Starting go-argo-lite application...")

	// Define path for SyncTarget storage file
	syncTargetsFile := DefaultSyncTargetsFile

	// Retrieve the encryption key
	// This logic is similar to what's in NewFileSyncTargetProvider and NewEncryptedFileStorage
	// We need a way to get the key bytes here for NewEncryptedFileStorage
	var encryptionKeyBytes []byte
	envKey := os.Getenv(EnvEncryptionKey)
	if envKey != "" {
		log.Printf("Using encryption key from environment variable %s", EnvEncryptionKey)
		encryptionKeyBytes = []byte(envKey)
	} else {
		log.Println("WARNING: Encryption key environment variable %s not set. Using hardcoded default key. This is INSECURE for production.", EnvEncryptionKey)
		// THIS IS INSECURE - Replace with a proper key management solution for production
		encryptionKeyBytes = []byte("0123456789abcdef0123456789abcdef") // 32-byte key for AES-256
	}

	if len(encryptionKeyBytes) != 16 && len(encryptionKeyBytes) != 24 && len(encryptionKeyBytes) != 32 {
		log.Fatalf("Encryption key must be 16, 24, or 32 bytes long, got %d bytes from %s or hardcoded default.", len(encryptionKeyBytes), EnvEncryptionKey)
	}


	// Initialize EncryptedFileStorage
	// Note: The storage path and key logic is now centralized here for main's setup.
	// The storage and provider constructors will use the key passed to them.
	dataStorage, err := storage.NewEncryptedFileStorage(syncTargetsFile, encryptionKeyBytes)
	if err != nil {
		log.Fatalf("Error initializing data storage: %v", err)
	}
	log.Println("Data storage initialized.")

	// Initialize the Worker
	appWorker := worker.NewWorker(dataStorage)
	log.Println("Worker initialized.")

	// Initialize the Server
	httpServer := server.NewServer(dataStorage, appWorker)
	log.Println("HTTP Server initialized.")

	// Start the worker
	// Worker's Start method should be non-blocking if it loads initial targets in goroutines.
	// (Current worker.Start() iterates and calls AddSyncTarget which launches goroutines)
	appWorker.Start()
	log.Println("Worker started.")

	// Start the server
	serverAddr := ":8080"
	log.Printf("Starting HTTP server on %s", serverAddr)
	go func() {
		if err := httpServer.Start(serverAddr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not start HTTP server: %v", err)
		}
	}()

	log.Println("Application started. Press Ctrl+C to exit.")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down application...")

	// Add graceful shutdown logic here for server and worker if needed.
	// For example:
	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()
	// if err := httpServer.Stop(ctx); err != nil { // Assuming server has a Stop method
	//    log.Printf("Error shutting down HTTP server: %v", err)
	// }
	// if err := appWorker.Stop(ctx); err != nil { // Assuming worker has a Stop method
	//    log.Printf("Error shutting down worker: %v", err)
	// }

	log.Println("Application shut down successfully.")
}
