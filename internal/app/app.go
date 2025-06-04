package app

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/go-argo-lite/internal/config"
	"github.com/user/go-argo-lite/internal/gitpoller"
	"github.com/user/go-argo-lite/internal/kubehandler"
)

// App orchestrates the git polling and Kubernetes manifest application.
type App struct {
	cfg         *config.Config
	poller      *gitpoller.GitPoller
	kubeHandler *kubehandler.KubeHandler
	// logger    *log.Logger // Using global log for now
}

// NewApp creates a new application instance.
func NewApp(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Define a local path for the git clone. This could be made configurable later.
	// For example, create a temporary directory or use a path from cfg.
	localRepoPath := "./.gitrepo" // TODO: Consider making this configurable or a temp dir

	poller, err := gitpoller.NewGitPoller(cfg.RepoURL, cfg.RepoBranch, localRepoPath, cfg.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitPoller: %w", err)
	}

	kubeHandler, err := kubehandler.NewKubeHandler(cfg.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create KubeHandler: %w", err)
	}

	log.Println("Application components initialized successfully.")
	return &App{
		cfg:         cfg,
		poller:      poller,
		kubeHandler: kubeHandler,
	}, nil
}

// Run starts the main application loop: polls the Git repository for changes
// and applies manifest files to Kubernetes if new commits are detected.
// It also handles graceful shutdown on interrupt signals.
func (a *App) Run() error {
	log.Println("Starting application run loop...")

	// Initial Repository Setup
	log.Printf("Initializing repository at %s (branch: %s)...", a.cfg.RepoURL, a.cfg.RepoBranch)
	if err := a.poller.InitializeRepo(); err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}
	log.Println("Repository initialized successfully.")

	// Setup ticker for polling interval
	ticker := time.NewTicker(time.Duration(a.cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Setup channel for OS signals for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting polling loop every %d seconds. Press Ctrl+C to exit.", a.cfg.PollIntervalSeconds)

	// Main application loop
	for {
		select {
		case <-ticker.C:
			log.Println("Polling for changes...")
			changed, commitHash, manifestFiles, err := a.poller.Poll()
			if err != nil {
				log.Printf("Error during repository poll: %v. Continuing...", err)
				// Depending on the error, might want to implement backoff or exit
				continue
			}

			if changed {
				log.Printf("Changes detected! New commit: %s", commitHash)
				if len(manifestFiles) == 0 {
					log.Printf("No manifest files found in '%s' for commit %s.", a.cfg.ManifestPath, commitHash)
				} else {
					log.Printf("Found %d manifest files to apply for commit %s:", len(manifestFiles), commitHash)
					for _, filePath := range manifestFiles {
						log.Printf(" - %s", filePath)
					}

					var applyErrors []string
					for _, filePath := range manifestFiles {
						log.Printf("Applying manifest: %s", filePath)
						if applyErr := a.kubeHandler.ApplyManifestFile(filePath); applyErr != nil {
							log.Printf("Error applying manifest %s: %v", filePath, applyErr)
							applyErrors = append(applyErrors, fmt.Sprintf("%s: %v", filePath, applyErr))
						} else {
							log.Printf("Successfully applied manifest: %s", filePath)
						}
					}

					if len(applyErrors) > 0 {
						log.Printf("Finished applying manifests for commit %s with %d error(s).", commitHash, len(applyErrors))
						// Potentially log details of applyErrors
					} else {
						log.Printf("All manifest files for commit %s applied successfully.", commitHash)
					}
				}
			} else {
				log.Printf("No new changes detected. Current commit: %s", commitHash)
			}

		case sig := <-signalChan:
			log.Printf("Received signal: %s. Shutting down gracefully...", sig)
			// Perform any cleanup here if necessary (e.g., delete localRepoPath)
			// For now, just exit.
			return nil
		}
	}
}
