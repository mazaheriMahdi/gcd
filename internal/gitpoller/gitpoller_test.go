package gitpoller

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/go-git/go-git/v5" // Import go-git
)

func TestNewGitPoller(t *testing.T) {
	t.Helper()
	t.Run("ValidInputs", func(t *testing.T) {
		poller, err := NewGitPoller("url", "branch", "path", "manifests")
		if err != nil {
			t.Fatalf("Expected no error for valid inputs, got %v", err)
		}
		if poller == nil {
			t.Fatal("Expected poller to be non-nil for valid inputs")
		}
		if poller.repoURL != "url" {
			t.Errorf("Expected repoURL 'url', got '%s'", poller.repoURL)
		}
		if poller.manifestPathInRepo != "manifests" {
			t.Errorf("Expected manifestPathInRepo 'manifests', got '%s'", poller.manifestPathInRepo)
		}
	})

	t.Run("InvalidInputs", func(t *testing.T) {
		_, err := NewGitPoller("", "branch", "path", "manifests")
		if err == nil {
			t.Error("Expected error for empty repoURL, got nil")
		}
		_, err = NewGitPoller("url", "", "path", "manifests")
		if err == nil {
			t.Error("Expected error for empty repoBranch, got nil")
		}
		_, err = NewGitPoller("url", "branch", "", "manifests")
		if err == nil {
			t.Error("Expected error for empty localPath, got nil")
		}
		_, err = NewGitPoller("url", "branch", "path", "")
		if err == nil {
			t.Error("Expected error for empty manifestPathInRepo, got nil")
		}
	})
}

func TestGetManifestFiles_Success(t *testing.T) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "gitpoller-test-success-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manifestsSubDir := "app-manifests"
	manifestPath := filepath.Join(tempDir, manifestsSubDir)
	if err := os.MkdirAll(manifestPath, 0755); err != nil {
		t.Fatalf("Failed to create manifests subdir: %v", err)
	}

	// Create some files
	expectedFiles := []string{
		filepath.Join(manifestPath, "deployment.yaml"),
		filepath.Join(manifestPath, "service.yml"),
	}
	otherFiles := []string{
		filepath.Join(manifestPath, "README.md"),
		filepath.Join(tempDir, "some-other-file.txt"), // File outside manifestPathInRepo
	}

	for _, f := range expectedFiles {
		if _, err := os.Create(f); err != nil {
			t.Fatalf("Failed to create test file %s: %v", f, err)
		}
	}
	for _, f := range otherFiles {
		if _, err := os.Create(f); err != nil {
			t.Fatalf("Failed to create test file %s: %v", f, err)
		}
	}
	// Create a nested directory with a yaml file inside manifestsSubDir
	nestedDir := filepath.Join(manifestPath, "nested")
	if err := os.Mkdir(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}
	expectedFiles = append(expectedFiles, filepath.Join(nestedDir, "configmap.yaml"))
	if _, err := os.Create(filepath.Join(nestedDir, "configmap.yaml")); err != nil {
		t.Fatalf("Failed to create nested test file: %v", err)
	}


	// GitPoller needs a repository, but GetManifestFiles only works on localPath.
	// We are not testing git operations here, so a nil repository is fine for this specific method.
	// Provide a dummy non-nil repository object to satisfy the initial check in GetManifestFiles.
	// The method's logic being tested here doesn't actually use the repository object itself.
	dummyRepo := &git.Repository{}
	gp := &GitPoller{
		localPath:          tempDir, // Root of the "cloned" repo
		manifestPathInRepo: manifestsSubDir,
		repository:         dummyRepo,
	}

	files, err := gp.GetManifestFiles()
	if err != nil {
		t.Fatalf("GetManifestFiles() returned an error: %v", err)
	}

	// Sort both slices for consistent comparison
	sort.Strings(files)
	sort.Strings(expectedFiles)

	if !reflect.DeepEqual(files, expectedFiles) {
		t.Errorf("GetManifestFiles() returned %v, expected %v", files, expectedFiles)
	}
}


func TestGetManifestFiles_NoManifestDir(t *testing.T) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "gitpoller-test-nodir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dummyRepo := &git.Repository{}
	gp := &GitPoller{
		localPath:          tempDir,
		manifestPathInRepo: "nonexistent-manifests-dir",
		repository:         dummyRepo,
	}

	files, err := gp.GetManifestFiles()
	// Expect an error because the directory itself doesn't exist.
	if err == nil {
		t.Errorf("GetManifestFiles() was expected to return an error for a non-existent manifest directory, but got nil. Files: %v", files)
	}
	// The error message should indicate the path was not found.
	// Contains check is more robust than exact match.
	// expectedErrorSubString := "not found in repository"
	// if err != nil && !strings.Contains(err.Error(), expectedErrorSubString) {
	// 	t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorSubString, err.Error())
	// }

	if len(files) != 0 {
		t.Errorf("Expected empty file list when manifest dir is missing, got %d files", len(files))
	}
}

func TestGetManifestFiles_EmptyManifestDir(t *testing.T) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "gitpoller-test-emptydir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manifestsSubDir := "empty-manifests"
	manifestPath := filepath.Join(tempDir, manifestsSubDir)
	if err := os.MkdirAll(manifestPath, 0755); err != nil {
		t.Fatalf("Failed to create empty manifests subdir: %v", err)
	}

	dummyRepo := &git.Repository{}
	gp := &GitPoller{
		localPath:          tempDir,
		manifestPathInRepo: manifestsSubDir,
		repository:         dummyRepo,
	}

	files, err := gp.GetManifestFiles()
	if err != nil {
		t.Fatalf("GetManifestFiles() returned an error for an empty manifest directory: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected empty file list for an empty manifest directory, got %d files: %v", len(files), files)
	}
}
