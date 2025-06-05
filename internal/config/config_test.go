package config

import (
	"os"
	"strconv"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	t.Helper()
	// Set environment variables
	originalRepoURL := os.Getenv("REPO_URL")
	originalRepoBranch := os.Getenv("REPO_BRANCH")
	originalPollInterval := os.Getenv("POLL_INTERVAL_SECONDS")
	originalManifestPath := os.Getenv("MANIFEST_PATH")
	originalKubeconfigPath := os.Getenv("KUBECONFIG_PATH")

	testRepoURL := "https://git.example.com/repo.git"
	testRepoBranch := "main"
	testPollInterval := "120"
	testManifestPath := "k8s/overlays/prod"
	testKubeconfigPath := "/tmp/test-kubeconfig"

	os.Setenv("REPO_URL", testRepoURL)
	os.Setenv("REPO_BRANCH", testRepoBranch)
	os.Setenv("POLL_INTERVAL_SECONDS", testPollInterval)
	os.Setenv("MANIFEST_PATH", testManifestPath)
	os.Setenv("KUBECONFIG_PATH", testKubeconfigPath)

	// Unset them after the test
	defer func() {
		os.Setenv("REPO_URL", originalRepoURL)
		os.Setenv("REPO_BRANCH", originalRepoBranch)
		os.Setenv("POLL_INTERVAL_SECONDS", originalPollInterval)
		os.Setenv("MANIFEST_PATH", originalManifestPath)
		os.Setenv("KUBECONFIG_PATH", originalKubeconfigPath)
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned an unexpected error: %v", err)
	}

	if cfg.RepoURL != testRepoURL {
		t.Errorf("expected RepoURL %s, got %s", testRepoURL, cfg.RepoURL)
	}
	if cfg.RepoBranch != testRepoBranch {
		t.Errorf("expected RepoBranch %s, got %s", testRepoBranch, cfg.RepoBranch)
	}
	expectedPollInt, _ := strconv.Atoi(testPollInterval)
	if cfg.PollIntervalSeconds != expectedPollInt {
		t.Errorf("expected PollIntervalSeconds %d, got %d", expectedPollInt, cfg.PollIntervalSeconds)
	}
	if cfg.ManifestPath != testManifestPath {
		t.Errorf("expected ManifestPath %s, got %s", testManifestPath, cfg.ManifestPath)
	}
	if cfg.KubeconfigPath != testKubeconfigPath {
		t.Errorf("expected KubeconfigPath %s, got %s", testKubeconfigPath, cfg.KubeconfigPath)
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	t.Helper()
	// Ensure REPO_URL is not set
	originalRepoURL := os.Getenv("REPO_URL")
	originalRepoBranch := os.Getenv("REPO_BRANCH")
	os.Unsetenv("REPO_URL")
	os.Setenv("REPO_BRANCH", "main") // Set other required vars

	defer func() {
		os.Setenv("REPO_URL", originalRepoURL)
		os.Setenv("REPO_BRANCH", originalRepoBranch)
	}()

	cfg, err := LoadConfig()
	if err == nil {
		t.Fatalf("LoadConfig() was expected to return an error for missing REPO_URL, but it didn't. Config: %+v", cfg)
	}
	// Check if the error message is somewhat relevant (optional)
	expectedErrorMsg := "REPO_URL environment variable is required"
	if err.Error() != expectedErrorMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}

	// Test missing REPO_BRANCH
	os.Setenv("REPO_URL", "https://git.example.com/repo.git")
	os.Unsetenv("REPO_BRANCH")
	cfg, err = LoadConfig()
	if err == nil {
		t.Fatalf("LoadConfig() was expected to return an error for missing REPO_BRANCH, but it didn't. Config: %+v", cfg)
	}
	expectedErrorMsg = "REPO_BRANCH environment variable is required"
	if err.Error() != expectedErrorMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Helper()
	originalRepoURL := os.Getenv("REPO_URL")
	originalRepoBranch := os.Getenv("REPO_BRANCH")
	originalPollInterval := os.Getenv("POLL_INTERVAL_SECONDS")
	originalManifestPath := os.Getenv("MANIFEST_PATH")
	originalKubeconfigPath := os.Getenv("KUBECONFIG_PATH")

	testRepoURL := "https://git.example.com/repo.git"
	testRepoBranch := "develop"

	os.Setenv("REPO_URL", testRepoURL)
	os.Setenv("REPO_BRANCH", testRepoBranch)
	// Unset optional variables to test defaults
	os.Unsetenv("POLL_INTERVAL_SECONDS")
	os.Unsetenv("MANIFEST_PATH")
	os.Unsetenv("KUBECONFIG_PATH")

	defer func() {
		os.Setenv("REPO_URL", originalRepoURL)
		os.Setenv("REPO_BRANCH", originalRepoBranch)
		os.Setenv("POLL_INTERVAL_SECONDS", originalPollInterval)
		os.Setenv("MANIFEST_PATH", originalManifestPath)
		os.Setenv("KUBECONFIG_PATH", originalKubeconfigPath)
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned an unexpected error: %v", err)
	}

	defaultPollInterval := 60
	if cfg.PollIntervalSeconds != defaultPollInterval {
		t.Errorf("expected default PollIntervalSeconds %d, got %d", defaultPollInterval, cfg.PollIntervalSeconds)
	}

	defaultManifestPath := "manifests"
	if cfg.ManifestPath != defaultManifestPath {
		t.Errorf("expected default ManifestPath '%s', got '%s'", defaultManifestPath, cfg.ManifestPath)
	}

	if cfg.KubeconfigPath != "" { // Default for KubeconfigPath is empty string
		t.Errorf("expected default KubeconfigPath to be empty, got '%s'", cfg.KubeconfigPath)
	}
}

func TestLoadConfig_InvalidPollInterval(t *testing.T) {
	t.Helper()
	originalRepoURL := os.Getenv("REPO_URL")
	originalRepoBranch := os.Getenv("REPO_BRANCH")
	originalPollInterval := os.Getenv("POLL_INTERVAL_SECONDS")

	os.Setenv("REPO_URL", "https://git.example.com/repo.git")
	os.Setenv("REPO_BRANCH", "main")
	os.Setenv("POLL_INTERVAL_SECONDS", "not-an-integer")

	defer func() {
		os.Setenv("REPO_URL", originalRepoURL)
		os.Setenv("REPO_BRANCH", originalRepoBranch)
		os.Setenv("POLL_INTERVAL_SECONDS", originalPollInterval)
	}()

	cfg, err := LoadConfig()
	if err == nil {
		t.Fatalf("LoadConfig() was expected to return an error for invalid POLL_INTERVAL_SECONDS, but it didn't. Config: %+v", cfg)
	}
	expectedErrorMsg := "POLL_INTERVAL_SECONDS must be a valid integer"
	if err.Error() != expectedErrorMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}
