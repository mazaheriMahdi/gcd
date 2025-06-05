package config

import (
	"errors"
	"os"
	"strconv"
)

// Config holds the application configuration, loaded from environment variables.
type Config struct {
	RepoURL             string
	RepoBranch          string
	KubeconfigPath      string
	PollIntervalSeconds int
	ManifestPath        string
}

// LoadConfig loads configuration from environment variables.
// It returns a Config struct and an error if required variables are missing
// or if there's an issue parsing them.
func LoadConfig() (*Config, error) {
	repoURL := os.Getenv("REPO_URL")
	if repoURL == "" {
		return nil, errors.New("REPO_URL environment variable is required")
	}

	repoBranch := os.Getenv("REPO_BRANCH")
	if repoBranch == "" {
		return nil, errors.New("REPO_BRANCH environment variable is required")
	}

	kubeconfigPath := os.Getenv("KUBECONFIG_PATH") // Optional

	pollIntervalStr := os.Getenv("POLL_INTERVAL_SECONDS")
	pollIntervalSeconds := 60 // Default value
	if pollIntervalStr != "" {
		var err error
		pollIntervalSeconds, err = strconv.Atoi(pollIntervalStr)
		if err != nil {
			return nil, errors.New("POLL_INTERVAL_SECONDS must be a valid integer")
		}
	}

	manifestPath := os.Getenv("MANIFEST_PATH")
	if manifestPath == "" {
		manifestPath = "manifests" // Default value
	}

	return &Config{
		RepoURL:             repoURL,
		RepoBranch:          repoBranch,
		KubeconfigPath:      kubeconfigPath,
		PollIntervalSeconds: pollIntervalSeconds,
		ManifestPath:        manifestPath,
	}, nil
}
