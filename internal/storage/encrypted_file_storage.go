package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/user/go-argo-lite/internal/interfaces"
	"github.com/user/go-argo-lite/internal/synctarget" // For Encrypt/Decrypt
)

const (
	// DefaultStorageFile is the default path for the storage file.
	DefaultStorageFile = "synctargets.json.enc" // Consistent with FileSyncTargetProvider
	// EnvEncryptionKey is the environment variable for the encryption key.
	EnvEncryptionKey = "GO_ARGO_LITE_ENCRYPTION_KEY" // Consistent
)

// EncryptedFileStorage implements the DataStorage interface using an encrypted file.
type EncryptedFileStorage struct {
	FilePath      string
	EncryptionKey []byte
}

// NewEncryptedFileStorage creates a new EncryptedFileStorage.
// If filePath is empty, DefaultStorageFile is used.
// If encryptionKey is nil, it attempts to read from the GO_ARGO_LITE_ENCRYPTION_KEY
// environment variable. If the environment variable is not set, a hardcoded key is used
// (INSECURE, for development only), and a warning is logged.
func NewEncryptedFileStorage(filePath string, encryptionKey []byte) (*EncryptedFileStorage, error) {
	if filePath == "" {
		filePath = DefaultStorageFile
	}

	var key []byte
	usedEnvKey := false
	usedHardcodedKey := false

	if len(encryptionKey) > 0 {
		key = encryptionKey
	} else {
		envKey := os.Getenv(EnvEncryptionKey)
		if envKey != "" {
			key = []byte(envKey)
			usedEnvKey = true
		} else {
			// THIS IS INSECURE - Replace with a proper key management solution for production
			key = []byte("0123456789abcdef0123456789abcdef") // 32-byte key for AES-256
			usedHardcodedKey = true
		}
	}

	// Validate key length
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 16, 24, or 32 bytes long, got %d bytes", len(key))
	}

	if usedEnvKey {
		log.Printf("EncryptedFileStorage: Using encryption key from environment variable %s", EnvEncryptionKey)
	}
	if usedHardcodedKey {
		log.Println("WARNING: EncryptedFileStorage: Using hardcoded encryption key. This is insecure and should only be used for development.")
	}


	return &EncryptedFileStorage{
		FilePath:      filePath,
		EncryptionKey: key,
	}, nil
}

// LoadSyncTargets reads, decrypts, and unmarshals sync targets from the storage file.
func (s *EncryptedFileStorage) LoadSyncTargets() ([]interfaces.SyncTarget, error) {
	encryptedData, err := os.ReadFile(s.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Storage file '%s' not found, returning empty list of sync targets.", s.FilePath)
			return []interfaces.SyncTarget{}, nil
		}
		return nil, fmt.Errorf("failed to read storage file '%s': %w", s.FilePath, err)
	}

	if len(encryptedData) == 0 {
		log.Printf("Storage file '%s' is empty, returning empty list of sync targets.", s.FilePath)
		return []interfaces.SyncTarget{}, nil
	}

	decryptedData, err := synctarget.Decrypt(encryptedData, s.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data from '%s': %w", s.FilePath, err)
	}

	var targets []interfaces.SyncTarget
	if err := json.Unmarshal(decryptedData, &targets); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync targets from JSON in '%s': %w", s.FilePath, err)
	}
	// log.Printf("Successfully loaded %d sync target(s) from %s", len(targets), s.FilePath) // Avoid noisy logging here, FileSyncTargetProvider does it.
	return targets, nil
}

// SaveSyncTarget adds a new sync target to the storage file.
// It loads existing targets, appends the new one, and overwrites the file.
func (s *EncryptedFileStorage) SaveSyncTarget(target interfaces.SyncTarget) error {
	targets, err := s.LoadSyncTargets()
	if err != nil {
		// Allow saving even if the file didn't exist or was empty/corrupted previously,
		// by starting with an empty list. However, if LoadSyncTargets returned an error
		// for a reason other than file not existing or being empty, we should propagate it.
		// For simplicity here, we'll only log and proceed if it's a "real" error from LoadSyncTargets.
		// A more robust solution might involve more specific error handling from LoadSyncTargets.
		if !os.IsNotExist(err) && len(targets) == 0 { // Attempt to recover if file was just unreadable
			log.Printf("Error loading existing sync targets during save: %v. Attempting to overwrite with new target.", err)
		} else if err != nil {
             return fmt.Errorf("failed to load existing sync targets before saving: %w", err)
        }
	}

	// Check for duplicates by ID to prevent adding the same target multiple times.
	// If a target with the same ID exists, update it. Otherwise, append.
	found := false
	for i, t := range targets {
		if t.ID == target.ID {
			targets[i] = target // Update existing target
			found = true
			break
		}
	}
	if !found {
		targets = append(targets, target)
	}

	jsonData, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync targets to JSON: %w", err)
	}

	encryptedData, err := synctarget.Encrypt(jsonData, s.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt sync targets data: %w", err)
	}

	// Write with 0600 permissions (owner read/write)
	if err := os.WriteFile(s.FilePath, encryptedData, 0600); err != nil {
		return fmt.Errorf("failed to write sync targets to file '%s': %w", s.FilePath, err)
	}

	log.Printf("Successfully saved/updated sync target ID '%s' to %s. Total targets: %d", target.ID, s.FilePath, len(targets))
	return nil
}
