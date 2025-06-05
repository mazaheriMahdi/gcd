package interfaces

// SyncTarget represents a target for synchronization.
type SyncTarget struct {
	ID                  string
	RepoURL             string
	RepoBranch          string
	KubeConfigContent   string
	PollIntervalSeconds int
	GitCredentials      string // Or a more structured type for credentials
	ManifestPath        string
}

// SyncTargetProvider defines the interface for loading sync targets.
type SyncTargetProvider interface {
	LoadSyncTargets() ([]SyncTarget, error)
}

// GitPoller defines the interface for polling a Git repository.
type GitPoller interface {
	Poll() (changed bool, commitHash string, manifestFiles []string, err error)
	GetManifestFiles() ([]string, error)
	InitializeRepo() error
}

// KubeHandler defines the interface for interacting with Kubernetes.
type KubeHandler interface {
	ApplyManifestFile(filePath string) error
}

// DataStorage defines the interface for storing and retrieving sync target data.
type DataStorage interface {
	SaveSyncTarget(target SyncTarget) error
	LoadSyncTargets() ([]SyncTarget, error)
}
