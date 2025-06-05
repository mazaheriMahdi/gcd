package worker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/user/go-argo-lite/internal/gitpoller"
	"github.com/user/go-argo-lite/internal/interfaces"
	"github.com/user/go-argo-lite/internal/kubehandler"
)

const (
	baseRepoPath = "/tmp/go-argo-lite-repos" // Base directory for cloning repos
)

// ManagedSyncTarget holds a SyncTarget and its stop channel
type ManagedSyncTarget struct {
	Target     interfaces.SyncTarget
	StopChan   chan struct{}
	Poller     interfaces.GitPoller // Keep a reference if needed for other operations
	KubeClient interfaces.KubeHandler // Keep a reference if needed
}

// Worker manages multiple SyncTargets, polling them for changes and applying manifests.
type Worker struct {
	dataStorage    interfaces.DataStorage
	managedTargets map[string]*ManagedSyncTarget // Keyed by SyncTarget.ID
	mu             sync.RWMutex                  // To protect access to managedTargets
}

// NewWorker creates a new Worker instance.
func NewWorker(dataStorage interfaces.DataStorage) *Worker {
	err := os.MkdirAll(baseRepoPath, 0750) // Ensure base path exists
	if err != nil && !os.IsExist(err) {
		log.Printf("Warning: Could not create base repository path %s: %v", baseRepoPath, err)
		// Depending on requirements, might want to return an error here
	}

	return &Worker{
		dataStorage:    dataStorage,
		managedTargets: make(map[string]*ManagedSyncTarget),
	}
}

// Start loads initial SyncTargets and begins managing them.
func (w *Worker) Start() {
	log.Println("Worker starting...")
	targets, err := w.dataStorage.LoadSyncTargets()
	if err != nil {
		log.Printf("Error loading initial sync targets: %v", err)
		// Depending on policy, might want to panic or exit
		return
	}

	log.Printf("Loaded %d sync targets from data storage.", len(targets))
	for _, target := range targets {
		// Need to pass a copy of target to the goroutine
		// as the loop variable 'target' will change
		tCopy := target
		if err := w.AddSyncTarget(tCopy); err != nil {
			log.Printf("Error starting management for initial target ID %s: %v", tCopy.ID, err)
		}
	}
	log.Println("Worker finished processing initial sync targets.")
}

// AddSyncTarget adds a new sync target to the worker and starts its management goroutine.
// It returns an error if a target with the same ID is already being managed.
func (w *Worker) AddSyncTarget(target interfaces.SyncTarget) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.managedTargets[target.ID]; exists {
		log.Printf("Sync target with ID '%s' is already being managed.", target.ID)
		return fmt.Errorf("target ID '%s' already managed", target.ID)
	}

	// It's crucial that KubeConfigContent is correctly populated in the target
	if target.KubeConfigContent == "" {
		log.Printf("Warning: SyncTarget ID '%s' has no KubeConfigContent. KubeHandler might use default paths or in-cluster config.", target.ID)
	}

	mst := &ManagedSyncTarget{
		Target:   target,
		StopChan: make(chan struct{}),
	}
	w.managedTargets[target.ID] = mst

	log.Printf("Starting management for sync target ID '%s' (URL: %s, Branch: %s, Path: %s)",
		target.ID, target.RepoURL, target.RepoBranch, target.ManifestPath)

	go w.manageSyncTarget(mst)
	return nil
}

// RemoveSyncTarget stops management of a sync target and removes it. (TODO for later)
// func (w *Worker) RemoveSyncTarget(targetID string) error { ... }

// manageSyncTarget is the core logic for a single sync target.
// It runs in its own goroutine.
func (w *Worker) manageSyncTarget(mst *ManagedSyncTarget) {
	target := mst.Target
	repoPath := filepath.Join(baseRepoPath, target.ID) // Unique path for this target's repo clone

	log.Printf("[%s] Initializing KubeHandler...", target.ID)
	// Pass empty string for kubeconfigPath if KubeConfigContent is provided
	kubeClient, err := kubehandler.NewKubeHandler("", []byte(target.KubeConfigContent))
	if err != nil {
		log.Printf("[%s] Error creating KubeHandler: %v. Goroutine will not start.", target.ID, err)
		// Optionally, remove from managedTargets or mark as failed
		w.mu.Lock()
		delete(w.managedTargets, target.ID)
		w.mu.Unlock()
		return
	}
	mst.KubeClient = kubeClient // Store for potential future use

	log.Printf("[%s] Initializing GitPoller for repo %s (branch %s) at %s, manifest path: %s",
		target.ID, target.RepoURL, target.RepoBranch, repoPath, target.ManifestPath)
	poller, err := gitpoller.NewGitPoller(target.RepoURL, target.RepoBranch, repoPath, target.ManifestPath)
	if err != nil {
		log.Printf("[%s] Error creating GitPoller: %v. Goroutine will not start.", target.ID, err)
		w.mu.Lock()
		delete(w.managedTargets, target.ID)
		w.mu.Unlock()
		return
	}
	mst.Poller = poller // Store for potential future use

	log.Printf("[%s] Initializing repository...", target.ID)
	if err := poller.InitializeRepo(); err != nil {
		log.Printf("[%s] Error initializing repository: %v. Goroutine will not start.", target.ID, err)
		w.mu.Lock()
		delete(w.managedTargets, target.ID)
		w.mu.Unlock()
		return
	}
	log.Printf("[%s] Repository initialized successfully.", target.ID)

	// Initial poll to set baseline and apply once on startup
	log.Printf("[%s] Performing initial poll and apply...", target.ID)
	changed, commitHash, manifestFiles, pollErr := poller.Poll()
	if pollErr != nil {
		log.Printf("[%s] Error during initial poll: %v", target.ID, pollErr)
		// Decide if to continue or exit; for now, we log and continue to periodic polling
	}
	if changed {
		log.Printf("[%s] Initial poll detected changes (commit: %s). Applying %d manifest(s)...", target.ID, commitHash, len(manifestFiles))
		for _, mf := range manifestFiles {
			log.Printf("[%s] Applying manifest: %s", target.ID, mf)
			if err := kubeClient.ApplyManifestFile(mf); err != nil {
				log.Printf("[%s] Error applying manifest %s: %v", target.ID, mf, err)
			} else {
				log.Printf("[%s] Successfully applied manifest: %s", target.ID, mf)
			}
		}
	} else if pollErr == nil {
		log.Printf("[%s] Initial poll found no changes or manifests already up to date.", target.ID)
	}


	pollInterval := time.Duration(target.PollIntervalSeconds) * time.Second
	if pollInterval <= 0 {
		log.Printf("[%s] Invalid poll interval %d, defaulting to 60 seconds", target.ID, target.PollIntervalSeconds)
		pollInterval = 60 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Printf("[%s] Starting polling loop with interval %s", target.ID, pollInterval.String())

	for {
		select {
		case <-ticker.C:
			log.Printf("[%s] Polling for changes...", target.ID)
			changed, commitHash, manifestFiles, err := poller.Poll()
			if err != nil {
				log.Printf("[%s] Error during poll: %v", target.ID, err)
				continue // Continue to next tick
			}

			if changed {
				log.Printf("[%s] Changes detected (new commit: %s). Applying %d manifest(s)...", target.ID, commitHash, len(manifestFiles))
				for _, mf := range manifestFiles {
					log.Printf("[%s] Applying manifest: %s", target.ID, mf)
					if err := kubeClient.ApplyManifestFile(mf); err != nil {
						log.Printf("[%s] Error applying manifest %s: %v", target.ID, mf, err)
						// Consider if partial failure should stop further applies in this batch
					} else {
						log.Printf("[%s] Successfully applied manifest: %s", target.ID, mf)
					}
				}
			} else {
				log.Printf("[%s] No changes detected.", target.ID)
			}

		case <-mst.StopChan:
			log.Printf("[%s] Stop signal received. Exiting management goroutine.", target.ID)
			// Cleanup: remove the cloned repository
			log.Printf("[%s] Cleaning up repository at %s", target.ID, repoPath)
			if err := os.RemoveAll(repoPath); err != nil {
				log.Printf("[%s] Error cleaning up repository %s: %v", target.ID, repoPath, err)
			}
			return
		}
	}
}
