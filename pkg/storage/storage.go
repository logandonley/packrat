package storage

// Storage defines the interface for backup storage implementations
type Storage interface {
	// Upload uploads a file to the storage
	Upload(localPath, remoteName string) error

	// Download downloads a file from the storage
	Download(remoteName, localPath string) error

	// List lists all backup files in the storage with the given prefix
	List(prefix string) ([]BackupFile, error)

	// Delete deletes a file from the storage
	Delete(remoteName string) error

	// Close closes any open connections
	Close() error
}

// Factory creates a new storage instance based on configuration
type Factory interface {
	// Create creates a new storage instance
	Create() (Storage, error)
}
