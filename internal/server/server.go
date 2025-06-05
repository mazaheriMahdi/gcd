package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/user/go-argo-lite/internal/interfaces"
	"github.com/user/go-argo-lite/internal/worker"
)

// Server handles HTTP requests for managing SyncTargets.
type Server struct {
	dataStorage interfaces.DataStorage
	worker      *worker.Worker
	router      *http.ServeMux
}

// NewServer creates a new Server instance and sets up its routes.
func NewServer(dataStorage interfaces.DataStorage, worker *worker.Worker) *Server {
	s := &Server{
		dataStorage: dataStorage,
		worker:      worker,
		router:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.router.HandleFunc("POST /sync-targets", s.handleCreateSyncTarget())
	// s.router.HandleFunc("GET /sync-targets", s.handleGetSyncTargets()) // Example for later
	// s.router.HandleFunc("DELETE /sync-targets/{id}", s.handleDeleteSyncTarget()) // Example for later

	// UI serving
	// Serve index.html at the root
	s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "ui/static/index.html")
		} else if r.URL.Path == "/favicon.ico" { // Optional: handle favicon explicitly or let it 404 if not present
			http.NotFound(w, r) // Or serve a favicon if you have one
		} else {
			// If you want to make it a Single Page Application (SPA) where unknown paths also serve index.html:
			// http.ServeFile(w, r, "ui/static/index.html")
			// Otherwise, for non-SPA, a 404 is appropriate for unhandled paths.
			http.NotFound(w, r)
		}
	})

	// Serve other static files (CSS, JS, images) from ui/static under /static/ prefix
	// e.g., /static/style.css would serve ui/static/style.css
	fs := http.FileServer(http.Dir("ui/static"))
	s.router.Handle("/static/", http.StripPrefix("/static/", fs))

	// Add other routes here:
	// s.router.HandleFunc("GET /sync-targets", s.handleGetSyncTargets())
	// s.router.HandleFunc("DELETE /sync-targets/{id}", s.handleDeleteSyncTarget())
}

// Start begins listening for HTTP requests on the given address.
func (s *Server) Start(address string) error {
	log.Printf("HTTP server starting on %s", address)
	// Start the server in a goroutine so it doesn't block
	go func() {
		if err := http.ListenAndServe(address, s.router); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not start HTTP server: %v", err)
		}
	}()
	return nil // Or return an error if initial setup fails before ListenAndServe
}

// handleCreateSyncTarget handles requests to create a new SyncTarget.
func (s *Server) handleCreateSyncTarget() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var target interfaces.SyncTarget
		if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
			log.Printf("Error decoding request body: %v", err)
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Basic Validation
		if target.RepoURL == "" {
			http.Error(w, "RepoURL is required", http.StatusBadRequest)
			return
		}
		if target.RepoBranch == "" {
			http.Error(w, "RepoBranch is required", http.StatusBadRequest)
			return
		}
		if target.KubeConfigContent == "" {
			// Depending on policy, could allow this if server has default kubeconfig access
			// and the user intends to use that. For now, making it mandatory for clarity.
			http.Error(w, "KubeConfigContent is required", http.StatusBadRequest)
			return
		}
		if target.ManifestPath == "" {
			http.Error(w, "ManifestPath is required", http.StatusBadRequest)
			return
		}
		if target.PollIntervalSeconds <= 0 {
			// Defaulting or asking for explicit value
			log.Printf("PollIntervalSeconds not set or invalid for new target (URL: %s), defaulting to 60s", target.RepoURL)
			target.PollIntervalSeconds = 60
		}

		// Generate ID
		target.ID = uuid.NewString()
		log.Printf("Generated new SyncTarget ID: %s for RepoURL: %s", target.ID, target.RepoURL)

		// Persist the target
		if err := s.dataStorage.SaveSyncTarget(target); err != nil {
			log.Printf("Error saving sync target ID %s: %v", target.ID, err)
			http.Error(w, fmt.Sprintf("Failed to save sync target: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("Successfully saved sync target ID %s to data storage.", target.ID)

		// Add to worker
		if err := s.worker.AddSyncTarget(target); err != nil {
			log.Printf("Error adding sync target ID %s to worker: %v", target.ID, err)
			// Note: The target is saved but worker failed to start. This might require compensation logic
			// or a status field in SyncTarget to indicate it's not actively running.
			// For now, we return an error to the client indicating partial failure.
			http.Error(w, fmt.Sprintf("Sync target saved, but failed to start processing: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("Successfully added sync target ID %s to worker.", target.ID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		response := map[string]string{"id": target.ID, "message": "SyncTarget created successfully"}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding success response for target ID %s: %v", target.ID, err)
		}
	}
}

// Consider adding a Stop method for graceful shutdown
func (s *Server) Stop(timeout time.Duration) error {
	// This would involve shutting down the http.Server gracefully
	// and potentially signaling the worker to stop its goroutines.
	log.Println("HTTP server stopping...")
	// Placeholder for actual graceful shutdown logic
	return nil
}
