package storage

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Debug controls verbose logging
var Debug bool

// debugLog prints a log message only if Debug is true
func debugLog(format string, v ...interface{}) {
	if Debug {
		log.Printf(format, v...)
	}
}

// SynologyConfig holds the configuration for Synology NAS storage
type SynologyConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	KeyFile  string `mapstructure:"key_file"`
	Path     string `mapstructure:"path"`
}

// SynologyStorage implements backup storage for Synology NAS
type SynologyStorage struct {
	config     *SynologyConfig
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

// NewSynologyStorage creates a new Synology storage instance
func NewSynologyStorage(config *SynologyConfig) (*SynologyStorage, error) {
	debugLog("Creating Synology storage with config: %+v", config)

	// Expand home directory in key file path if needed
	keyFile := config.KeyFile
	debugLog("Original key file path: %s", keyFile)
	if strings.HasPrefix(keyFile, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		keyFile = filepath.Join(home, keyFile[2:])
		debugLog("Expanded key file path: %s", keyFile)
	}

	// Read private key
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key file %s: %w", keyFile, err)
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}

	// Create SSH client config
	sshConfig := &ssh.ClientConfig{
		User: config.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
	}

	// Connect to the Synology NAS
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Synology NAS: %w", err)
	}

	// Create SFTP client
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}

	// Get current working directory
	pwd, err := sftpClient.Getwd()
	if err != nil {
		sftpClient.Close()
		sshClient.Close()
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	debugLog("Initial working directory: %s", pwd)

	return &SynologyStorage{
		config:     config,
		sshClient:  sshClient,
		sftpClient: sftpClient,
	}, nil
}

// Close closes the SFTP and SSH connections
func (s *SynologyStorage) Close() error {
	var errs []error
	if s.sftpClient != nil {
		if err := s.sftpClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close SFTP client: %w", err))
		}
	}
	if s.sshClient != nil {
		if err := s.sshClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close SSH client: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}
	return nil
}

// Upload uploads a file to the Synology NAS
func (s *SynologyStorage) Upload(localPath, remoteName string) error {
	debugLog("Starting upload: local=%s, remote=%s", localPath, remoteName)

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Get remote path and directory
	remotePath := s.getRemotePath(remoteName)
	remoteDir := filepath.Dir(remotePath)
	debugLog("Remote path: %s", remotePath)
	debugLog("Remote directory: %s", remoteDir)

	// Get current working directory
	pwd, err := s.sftpClient.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	debugLog("Current working directory: %s", pwd)

	// Create directory structure
	if err := s.mkdirAll(remoteDir); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// For file creation, use relative path if we're in home directory
	createPath := remotePath
	if strings.HasPrefix(remotePath, fmt.Sprintf("/volume1/homes/%s/", s.config.Username)) {
		homePath := fmt.Sprintf("/volume1/homes/%s/", s.config.Username)
		createPath = strings.TrimPrefix(remotePath, homePath)
		debugLog("Using relative path for file creation: %s", createPath)
	}

	// Create remote file
	remoteFile, err := s.sftpClient.Create(createPath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	// Copy file contents
	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	debugLog("Upload completed successfully")
	return nil
}

// BackupFile represents a backup file with its metadata
type BackupFile struct {
	// Name is the filename of the backup
	Name string
	// Size is the size of the backup file in bytes
	Size int64
	// ModTime is the modification time of the backup file in UTC
	ModTime string
}

// List lists all backup files in the storage
func (s *SynologyStorage) List(prefix string) ([]BackupFile, error) {
	// Get the base path without any filename
	path := s.getRemotePath("")
	debugLog("Listing files in directory: %s", path)

	// For listing, use path relative to SFTP root
	listPath := path
	if strings.HasPrefix(path, fmt.Sprintf("/volume1/homes/%s/", s.config.Username)) {
		// Instead of using the full path, use the path relative to SFTP root
		listPath = strings.TrimPrefix(path, fmt.Sprintf("/volume1/homes/%s/", s.config.Username))
		debugLog("Using SFTP root relative path for listing: %s", listPath)
	}

	files, err := s.sftpClient.ReadDir(listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote directory: %w", err)
	}

	debugLog("Found %d total files in directory", len(files))
	for _, file := range files {
		debugLog("Found file: %s (isDir: %v)", file.Name(), file.IsDir())
	}

	var backups []BackupFile
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), prefix) {
			debugLog("Found file matching prefix %s: %s", prefix, file.Name())
			backups = append(backups, BackupFile{
				Name:    file.Name(),
				Size:    file.Size(),
				ModTime: file.ModTime().UTC().Format("2006-01-02 15:04:05 UTC"),
			})
		}
	}

	debugLog("Found %d backup files", len(backups))
	return backups, nil
}

// Download downloads a file from the Synology NAS
func (s *SynologyStorage) Download(remoteName, localPath string) error {
	// Get remote path
	remotePath := s.getRemotePath(remoteName)
	debugLog("Remote path for download: %s", remotePath)

	// For file operations, use relative path if we're in home directory
	openPath := remotePath
	if strings.HasPrefix(remotePath, fmt.Sprintf("/volume1/homes/%s/", s.config.Username)) {
		homePath := fmt.Sprintf("/volume1/homes/%s/", s.config.Username)
		openPath = strings.TrimPrefix(remotePath, homePath)
		debugLog("Using relative path for download: %s", openPath)
	}

	// Open remote file
	remoteFile, err := s.sftpClient.Open(openPath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Copy file contents
	if _, err := io.Copy(localFile, remoteFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	return nil
}

// getRemotePath returns the full remote path for a given file name
func (s *SynologyStorage) getRemotePath(fileName string) string {
	path := s.config.Path
	debugLog("Original path from config: %s", path)

	// For relative paths, strip ./ prefix
	if strings.HasPrefix(path, "./") {
		path = path[2:]
	}

	// Join the base path with the filename
	result := path
	if fileName != "" {
		result = filepath.Join(path, fileName)
	}
	debugLog("Using path: %s", result)
	return result
}

// mkdirAll creates a directory and all parent directories if they don't exist
func (s *SynologyStorage) mkdirAll(path string) error {
	debugLog("mkdirAll called with path: %s", path)

	if path == "" || path == "." {
		debugLog("Empty or . path, returning")
		return nil
	}

	// Get current working directory
	pwd, err := s.sftpClient.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	debugLog("Current working directory before mkdir: %s", pwd)

	// If it's an absolute path and not in the home directory, return an error
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, fmt.Sprintf("/volume1/homes/%s", s.config.Username)) {
		return fmt.Errorf("path must be relative or within home directory")
	}

	// For absolute paths in home directory, make them relative
	if strings.HasPrefix(path, "/") {
		homePath := fmt.Sprintf("/volume1/homes/%s/", s.config.Username)
		path = strings.TrimPrefix(path, homePath)
		debugLog("Made path relative to home: %s", path)
	}

	// For relative paths, strip ./ prefix
	if strings.HasPrefix(path, "./") {
		path = path[2:]
		debugLog("Stripped ./ prefix: %s", path)
	}

	// Try to create the directory with MkdirAll first
	err = s.sftpClient.MkdirAll(path)
	if err == nil {
		debugLog("Successfully created directory path: %s", path)
		return nil
	}
	debugLog("MkdirAll failed, trying component by component: %v", err)

	// If MkdirAll fails, try component by component
	components := strings.Split(filepath.ToSlash(path), "/")
	debugLog("Path components: %v", components)

	current := ""
	for _, component := range components {
		if component == "" || component == "." {
			continue
		}

		if current == "" {
			current = component
		} else {
			current = filepath.Join(current, component)
		}

		debugLog("Attempting to create directory: %s", current)
		err := s.sftpClient.Mkdir(current)
		if err != nil {
			if !os.IsExist(err) {
				debugLog("Failed to create directory: %s, error: %v", current, err)
				return fmt.Errorf("failed to create directory %s: %w", current, err)
			}
			debugLog("Directory already exists: %s", current)
		} else {
			debugLog("Successfully created directory: %s", current)
		}
	}

	return nil
}

// Delete removes a backup file from storage
func (s *SynologyStorage) Delete(remoteName string) error {
	// Check current working directory
	pwd, err := s.sftpClient.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	debugLog("Current working directory: %s", pwd)

	// Get remote path and ensure it's absolute
	remotePath := s.getRemotePath(remoteName)
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}
	debugLog("Remote path for deletion: %s", remotePath)

	// Try to remove the file
	err = s.sftpClient.Remove(remotePath)
	if err != nil {
		debugLog("Error during deletion: %v", err)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	debugLog("Successfully deleted file")
	return nil
}
