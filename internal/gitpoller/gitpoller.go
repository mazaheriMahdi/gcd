package gitpoller

import (
	"fmt"
	"log"
	"os"
	"path/filepath" // For joining paths

	"github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config" // Renamed import
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	// "github.com/go-git/go-git/v5/plumbing/transport" // For auth if needed later
)

// GitPoller manages cloning and polling a git repository
type GitPoller struct {
	repoURL            string
	repoBranch         string
	localPath          string
	manifestPathInRepo string // e.g., "manifests" or "k8s"
	lastCommitHash     string
	repository         *git.Repository
	// auth           transport.AuthMethod // Optional: for private repositories
}

// NewGitPoller creates a new GitPoller instance
func NewGitPoller(repoURL, repoBranch, localPath, manifestPathInRepo string) (*GitPoller, error) {
	if repoURL == "" || repoBranch == "" || localPath == "" {
		return nil, fmt.Errorf("repoURL, repoBranch, and localPath must be provided")
	}
	if manifestPathInRepo == "" {
		// Or allow it to be empty and GetManifestFiles would return empty/error
		return nil, fmt.Errorf("manifestPathInRepo must be provided")
	}
	return &GitPoller{
		repoURL:            repoURL,
		repoBranch:         repoBranch,
		localPath:          localPath,
		manifestPathInRepo: manifestPathInRepo,
	}, nil
}

// InitializeRepo clones the repository if it doesn't exist, or opens it if it does.
// It also performs an initial checkout of the specified branch.
func (gp *GitPoller) InitializeRepo() error {
	// Check if the localPath exists and is a git repository
	_, err := os.Stat(filepath.Join(gp.localPath, ".git"))
	if os.IsNotExist(err) {
		// Path does not exist, clone the repository
		log.Printf("Cloning repository %s into %s\n", gp.repoURL, gp.localPath)
		r, err := git.PlainClone(gp.localPath, false, &git.CloneOptions{
			URL:           gp.repoURL,
			ReferenceName: plumbing.NewBranchReferenceName(gp.repoBranch),
			SingleBranch:  true,
			Progress:      os.Stdout, // Optional: for clone progress
		})
		if err != nil {
			return fmt.Errorf("failed to clone repository: %w", err)
		}
		gp.repository = r
		log.Println("Repository cloned successfully.")
	} else if err == nil {
		// Path exists, try to open it as a git repository
		log.Printf("Opening existing repository at %s\n", gp.localPath)
		r, err := git.PlainOpen(gp.localPath)
		if err != nil {
			return fmt.Errorf("failed to open existing repository: %w", err)
		}
		gp.repository = r
		log.Println("Repository opened successfully.")
		return gp.checkoutBranch()
	} else {
		return fmt.Errorf("error checking repository path %s: %w", gp.localPath, err)
	}
	return gp.checkoutBranch()
}

// checkoutBranch performs a checkout of the specified branch.
func (gp *GitPoller) checkoutBranch() error {
	if gp.repository == nil {
		return fmt.Errorf("repository not initialized")
	}

	w, err := gp.repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	branchRefName := plumbing.NewBranchReferenceName(gp.repoBranch)
	log.Printf("Attempting to checkout branch: %s\n", branchRefName)

	_, err = gp.repository.Reference(branchRefName, true)
	if err == plumbing.ErrReferenceNotFound {
		log.Printf("Branch %s not found locally, attempting to create from remote origin/%s\n", gp.repoBranch, gp.repoBranch)
		remoteBranchRefName := plumbing.NewRemoteReferenceName("origin", gp.repoBranch)
		headRef, err := gp.repository.Reference(remoteBranchRefName, true)
		if err != nil {
			return fmt.Errorf("remote branch %s not found: %w", remoteBranchRefName, err)
		}

		err = w.Checkout(&git.CheckoutOptions{
			Hash:   headRef.Hash(),
			Branch: branchRefName,
			Create: true,
		})
		if err != nil {
			return fmt.Errorf("failed to checkout new branch %s from remote: %w", gp.repoBranch, err)
		}
		log.Printf("Successfully checked out and created branch %s from %s\n", gp.repoBranch, remoteBranchRefName)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get reference for branch %s: %w", gp.repoBranch, err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Force:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", gp.repoBranch, err)
	}
	log.Printf("Successfully checked out branch %s\n", gp.repoBranch)
	return nil
}

// FetchLatest fetches the latest changes from the remote for the configured branch
// and resets the local branch to the fetched remote branch.
func (gp *GitPoller) FetchLatest() error {
	if gp.repository == nil {
		return fmt.Errorf("repository not initialized, call InitializeRepo first")
	}

	log.Printf("Fetching latest changes for branch %s from remote %s\n", gp.repoBranch, gp.repoURL)
	err := gp.repository.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []gogitconfig.RefSpec{gogitconfig.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", gp.repoBranch, gp.repoBranch))},
		Progress:   os.Stdout,
		Force:      true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch from remote: %w", err)
	}
	log.Println("Fetch completed.")

	w, err := gp.repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	remoteBranchRef := plumbing.NewRemoteReferenceName("origin", gp.repoBranch)
	targetRef, err := gp.repository.Reference(remoteBranchRef, true)
	if err != nil {
		return fmt.Errorf("failed to get reference for remote branch %s: %w", remoteBranchRef, err)
	}

	log.Printf("Resetting local branch %s to %s (%s)\n", gp.repoBranch, remoteBranchRef, targetRef.Hash())
	err = w.Reset(&git.ResetOptions{
		Commit: targetRef.Hash(),
		Mode:   git.HardReset,
	})
	if err != nil {
		return fmt.Errorf("failed to reset worktree to %s: %w", targetRef.Hash(), err)
	}

	return gp.checkoutBranch()
}

// GetCurrentCommitHash retrieves the commit hash of the current HEAD of the local working tree.
func (gp *GitPoller) GetCurrentCommitHash() (string, error) {
	if gp.repository == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	headRef, err := gp.repository.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD reference: %w", err)
	}
	return headRef.Hash().String(), nil
}

// GetManifestFiles scans the configured manifest directory within the local repository
// and returns a list of .yaml or .yml file paths.
func (gp *GitPoller) GetManifestFiles() ([]string, error) {
	if gp.repository == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	manifestDir := filepath.Join(gp.localPath, gp.manifestPathInRepo)
	log.Printf("Scanning for manifest files in: %s", manifestDir)

	var files []string
	err := filepath.WalkDir(manifestDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == manifestDir && d == nil {
				log.Printf("Manifest directory %s does not exist.", manifestDir)
				return fmt.Errorf("manifest directory '%s' not found in repository at path '%s'", gp.manifestPathInRepo, manifestDir)
			}
			return err
		}
		if !d.IsDir() {
			ext := filepath.Ext(d.Name())
			if ext == ".yaml" || ext == ".yml" {
				log.Printf("Found manifest file: %s", path)
				files = append(files, path)
			}
		}
		return nil
	})

	if err != nil {
		if _, statErr := os.Stat(manifestDir); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("manifest directory '%s' not found in repository at path '%s'", gp.manifestPathInRepo, manifestDir)
		}
		return nil, fmt.Errorf("error walking manifest directory %s: %w", manifestDir, err)
	}

	if len(files) == 0 {
		log.Printf("No manifest files (.yaml/.yml) found in %s", manifestDir)
	}
	return files, nil
}

// Poll checks for new commits. If a new commit is found, it fetches the changes,
// updates the local repository, updates lastCommitHash, retrieves manifest files, and returns true.
func (gp *GitPoller) Poll() (changed bool, commitHash string, manifestFiles []string, err error) {
	if gp.repository == nil {
		return false, "", nil, fmt.Errorf("repository not initialized, call InitializeRepo first")
	}

	log.Println("Polling for new commits...")

	fetchErr := gp.FetchLatest()
	if fetchErr != nil {
		return false, "", nil, fmt.Errorf("failed during fetch: %w", fetchErr)
	}

	newCommitHash, hashErr := gp.GetCurrentCommitHash()
	if hashErr != nil {
		return false, "", nil, fmt.Errorf("failed to get current commit hash: %w", hashErr)
	}

	if gp.lastCommitHash == "" { // First poll after initialization
		log.Printf("Initial commit hash for branch %s: %s\n", gp.repoBranch, newCommitHash)
		gp.lastCommitHash = newCommitHash

		files, listErr := gp.GetManifestFiles()
		if listErr != nil {
			return false, newCommitHash, nil, fmt.Errorf("failed to list manifest files on initial poll: %w", listErr)
		}
		return true, newCommitHash, files, nil
	}

	if newCommitHash != gp.lastCommitHash {
		log.Printf("New commit detected on branch %s. Old: %s, New: %s\n", gp.repoBranch, gp.lastCommitHash, newCommitHash)
		gp.lastCommitHash = newCommitHash

		files, listErr := gp.GetManifestFiles()
		if listErr != nil {
			return true, newCommitHash, nil, fmt.Errorf("new commit detected, but failed to list manifest files: %w", listErr)
		}
		return true, newCommitHash, files, nil
	}

	log.Printf("No new commits found on branch %s. Current hash: %s\n", gp.repoBranch, gp.lastCommitHash)
	return false, gp.lastCommitHash, nil, nil
}

// Helper function to get the *object.Commit from a hash string (if needed later)
func (gp *GitPoller) getCommitObject(hash string) (*object.Commit, error) {
	if gp.repository == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	commitHash := plumbing.NewHash(hash)
	return gp.repository.CommitObject(commitHash)
}
