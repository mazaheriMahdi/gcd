package synctarget

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/user/go-argo-lite/internal/interfaces"
)

const (
	// DefaultSyncTargetsFile is the default path for the sync targets JSON file.
	DefaultSyncTargetsFile = "synctargets.json.enc"
	// EnvEncryptionKey is the environment variable for the encryption key.
	EnvEncryptionKey = "GO_ARGO_LITE_ENCRYPTION_KEY"
)

// FileSyncTargetProvider implements the SyncTargetProvider interface
// by reading sync targets from an encrypted JSON file.
type FileSyncTargetProvider struct {
	FilePath      string
	EncryptionKey []byte
}

// NewFileSyncTargetProvider creates a new FileSyncTargetProvider.
// If filePath is empty, DefaultSyncTargetsFile is used.
// If encryptionKey is nil, it attempts to read from the GO_ARGO_LITE_ENCRYPTION_KEY
// environment variable. If the environment variable is not set, a hardcoded key is used
// (INSECURE, for development only).
func NewFileSyncTargetProvider(filePath string, encryptionKey []byte) (*FileSyncTargetProvider, error) {
	if filePath == "" {
		filePath = DefaultSyncTargetsFile
	}

	var key []byte
	if len(encryptionKey) > 0 {
		key = encryptionKey
	} else {
		envKey := os.Getenv(EnvEncryptionKey)
		if envKey != "" {
			key = []byte(envKey)
		} else {
			log.Println("WARNING: Using hardcoded encryption key. This is insecure and should only be used for development.")
			// THIS IS INSECURE - Replace with a proper key management solution for production
			key = []byte("0123456789abcdef0123456789abcdef") // 32-byte key for AES-256
		}
	}

	// Validate key length
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 16, 24, or 32 bytes long, got %d bytes", len(key))
	}

	return &FileSyncTargetProvider{
		FilePath:      filePath,
		EncryptionKey: key,
	}, nil
}

// LoadSyncTargets reads sync targets from the configured JSON file.
func (p *FileSyncTargetProvider) LoadSyncTargets() ([]interfaces.SyncTarget, error) {
	encryptedData, err := os.ReadFile(p.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// If the file doesn't exist, return an empty list of targets and no error.
			// This allows the application to start without a pre-existing file.
			log.Printf("Sync targets file '%s' not found, starting with no sync targets.", p.FilePath)
			return []interfaces.SyncTarget{}, nil
		}
		return nil, fmt.Errorf("failed to read sync targets file '%s': %w", p.FilePath, err)
	}

	// If the file is empty, return an empty list of targets.
	if len(encryptedData) == 0 {
		log.Printf("Sync targets file '%s' is empty, starting with no sync targets.", p.FilePath)
		return []interfaces.SyncTarget{}, nil
	}

	decryptedData, err := decrypt(encryptedData, p.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt sync targets data from '%s': %w", p.FilePath, err)
	}

	var targets []interfaces.SyncTarget
	if err := json.Unmarshal(decryptedData, &targets); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync targets from JSON in '%s': %w", p.FilePath, err)
	}

	log.Printf("Successfully loaded %d sync target(s) from %s", len(targets), p.FilePath)
	return targets, nil
}

// SaveSyncTargets is not part of SyncTargetProvider but is a utility for this package
// to write and encrypt targets to the file. This might be moved to a DataStorage implementation later.
func (p *FileSyncTargetProvider) SaveSyncTargets(targets []interfaces.SyncTarget) error {
	jsonData, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync targets to JSON: %w", err)
	}

	encryptedData, err := encrypt(jsonData, p.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt sync targets data: %w", err)
	}

	if err := os.WriteFile(p.FilePath, encryptedData, 0600); err != nil {
		return fmt.Errorf("failed to write sync targets file '%s': %w", p.FilePath, err)
	}
	log.Printf("Successfully saved %d sync target(s) to %s", len(targets), p.FilePath)
	return nil
}
