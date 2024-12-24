package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/klauspost/compress/zstd"
	"github.com/logandonley/packrat/pkg/config"
	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/logandonley/packrat/pkg/storage"
	"golang.org/x/sys/unix"
)

// isExcluded checks if a path matches any of the exclude patterns
func isExcluded(path string, excludePatterns []string) bool {
	// Convert path separators to forward slashes for consistent matching
	path = filepath.ToSlash(path)

	for _, pattern := range excludePatterns {
		// Convert pattern separators to forward slashes
		pattern = filepath.ToSlash(pattern)

		// Try to match the pattern
		matched, err := doublestar.Match(pattern, path)
		if err == nil && matched {
			return true
		}

		// Also check if any parent directory matches
		dir := path
		for dir != "." && dir != "/" {
			dir = filepath.Dir(dir)
			dir = filepath.ToSlash(dir)
			matched, err := doublestar.Match(pattern, dir)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

// Debug controls verbose logging
var Debug bool

// debugLog prints a log message only if Debug is true
func debugLog(format string, v ...interface{}) {
	if Debug {
		log.Printf(format, v...)
	}
}

// Manager handles backup operations
type Manager struct {
	config     *config.Config
	key        []byte
	dockerCli  *client.Client
	backupRoot string
	Synology   storage.Storage
	S3         storage.Storage
}

// NewManager creates a new backup manager
func NewManager(cfg *config.Config, key []byte) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	backupRoot := filepath.Join(os.TempDir(), "packrat-backups")
	if err := os.MkdirAll(backupRoot, 0700); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Create Synology storage
	synologyStorage, err := storage.NewSynologyStorage(&storage.SynologyConfig{
		Host:     cfg.Backup.Synology.Host,
		Port:     cfg.Backup.Synology.Port,
		Username: cfg.Backup.Synology.Username,
		KeyFile:  cfg.Backup.Synology.KeyFile,
		Path:     cfg.Backup.Synology.Path,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Synology storage: %w", err)
	}

	// Create S3 storage if configured
	var s3Storage storage.Storage
	if cfg.Backup.S3.Endpoint != "" {
		s3Storage, err = storage.NewS3Storage(&storage.S3Config{
			Endpoint:        cfg.Backup.S3.Endpoint,
			Region:          cfg.Backup.S3.Region,
			Bucket:          cfg.Backup.S3.Bucket,
			AccessKeyID:     cfg.Backup.S3.AccessKeyID,
			SecretAccessKey: cfg.Backup.S3.SecretAccessKey,
			Path:            cfg.Backup.S3.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create S3 storage: %w", err)
		}
	}

	return &Manager{
		config:     cfg,
		key:        key,
		dockerCli:  cli,
		backupRoot: backupRoot,
		Synology:   synologyStorage,
		S3:         s3Storage,
	}, nil
}

// Close closes all connections
func (m *Manager) Close() error {
	var errs []error
	if m.Synology != nil {
		if err := m.Synology.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Synology storage: %w", err))
		}
	}
	if m.S3 != nil {
		if err := m.S3.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close S3 storage: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to close storages: %v", errs)
	}
	return nil
}

// executeCommand executes a command with the specified configuration
func (m *Manager) executeCommand(cmd *config.Command, servicePath string) error {
	if cmd == nil {
		return nil
	}

	// Parse timeout duration
	var timeout time.Duration
	var err error
	if cmd.Timeout != "" {
		timeout, err = time.ParseDuration(cmd.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout duration: %w", err)
		}
	} else {
		timeout = 5 * time.Minute // Default timeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create command
	command := exec.CommandContext(ctx, "sh", "-c", cmd.Command)

	// Set working directory - use explicitly set directory or default to service path
	if cmd.WorkingDir != "" {
		command.Dir = cmd.WorkingDir
	} else {
		command.Dir = servicePath
	}

	// Set environment variables
	if len(cmd.Environment) > 0 {
		env := os.Environ()
		for key, value := range cmd.Environment {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		command.Env = env
	}

	// Capture output
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	debugLog("Command output: %s", string(output))
	return nil
}

// CreateBackup creates a backup of the specified service
func (m *Manager) CreateBackup(serviceName string) error {
	service, ok := m.config.Services[serviceName]
	if !ok {
		return fmt.Errorf("service %s not found in configuration", serviceName)
	}

	// Create temporary directory for the backup
	tmpDir := filepath.Join(m.backupRoot, fmt.Sprintf("%s-%d", serviceName, time.Now().Unix()))
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Execute pre-backup command if specified
	if service.PreBackup != nil {
		debugLog("Executing pre-backup command for service %s", serviceName)
		if err := m.executeCommand(service.PreBackup, service.Path); err != nil {
			return fmt.Errorf("failed to execute pre-backup command: %w", err)
		}
	}

	// Handle Docker container if specified
	if service.Docker != nil {
		if err := m.handleDockerContainer(service.Docker.Container, true); err != nil {
			return fmt.Errorf("failed to handle Docker container: %w", err)
		}
		defer m.handleDockerContainer(service.Docker.Container, false)
	}

	// Create tar.gz archive in memory
	archiveData := new(bytes.Buffer)
	if err := m.createArchive(service.Path, archiveData); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	// Encrypt the archive
	encrypted, err := crypto.Encrypt(m.key, archiveData.Bytes())
	if err != nil {
		return fmt.Errorf("failed to encrypt backup: %w", err)
	}

	// Create final backup name with timestamp
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	backupName := fmt.Sprintf("%s-%s.enc", serviceName, timestamp)

	// Save temporary local copy
	localPath := filepath.Join(tmpDir, backupName)
	if err := os.WriteFile(localPath, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to save backup locally: %w", err)
	}

	// Upload to Synology
	if err := m.Synology.Upload(localPath, backupName); err != nil {
		return fmt.Errorf("failed to upload to Synology: %w", err)
	}

	// Upload to S3 if configured
	if m.S3 != nil {
		if err := m.S3.Upload(localPath, backupName); err != nil {
			return fmt.Errorf("failed to upload to S3: %w", err)
		}
	}

	return nil
}

func (m *Manager) createArchive(sourcePath string, output io.Writer) error {
	zw, err := zstd.NewWriter(output)
	if err != nil {
		return fmt.Errorf("failed to create zstd writer: %w", err)
	}
	defer zw.Close()

	tw := tar.NewWriter(zw)
	defer tw.Close()

	// Get service configuration to access exclude patterns
	var excludePatterns []string
	for _, service := range m.config.Services {
		if strings.HasPrefix(sourcePath, service.Path) {
			excludePatterns = service.Exclude
			break
		}
	}

	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get path relative to source directory for pattern matching
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip excluded files/directories
		if isExcluded(relPath, excludePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}

		// Update header name to be relative to source directory
		header.Name = relPath

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// If it's a regular file, write the contents
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return fmt.Errorf("failed to write file contents: %w", err)
			}
		}

		return nil
	})
}

func (m *Manager) handleDockerContainer(containerName string, stop bool) error {
	ctx := context.Background()
	timeout := time.After(2 * time.Minute)

	if stop {
		log.Printf("Stopping Docker container: %s", containerName)
		timeoutSeconds := 30 // Give containers 30 seconds to stop gracefully
		if err := m.dockerCli.ContainerStop(ctx, containerName, container.StopOptions{Timeout: &timeoutSeconds}); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		// Wait for container to actually stop
		for {
			select {
			case <-timeout:
				return fmt.Errorf("timeout waiting for container %s to stop", containerName)
			default:
				info, err := m.dockerCli.ContainerInspect(ctx, containerName)
				if err != nil {
					return fmt.Errorf("failed to inspect container: %w", err)
				}
				if !info.State.Running {
					log.Printf("Container %s stopped successfully", containerName)
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	} else {
		log.Printf("Starting Docker container: %s", containerName)
		if err := m.dockerCli.ContainerStart(ctx, containerName, container.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		// Wait for container to be healthy if it has a healthcheck
		info, err := m.dockerCli.ContainerInspect(ctx, containerName)
		if err != nil {
			return fmt.Errorf("failed to inspect container: %w", err)
		}

		if info.State.Health != nil {
			log.Printf("Waiting for container %s to be healthy...", containerName)
			for {
				select {
				case <-timeout:
					return fmt.Errorf("timeout waiting for container %s to become healthy", containerName)
				default:
					info, err := m.dockerCli.ContainerInspect(ctx, containerName)
					if err != nil {
						return fmt.Errorf("failed to inspect container health: %w", err)
					}
					if info.State.Health.Status == "healthy" {
						log.Printf("Container %s is healthy", containerName)
						return nil
					}
					if info.State.Health.Status == "unhealthy" {
						return fmt.Errorf("container %s is unhealthy after start", containerName)
					}
					time.Sleep(1 * time.Second)
				}
			}
		} else {
			// If no healthcheck, just wait for it to be running
			for {
				select {
				case <-timeout:
					return fmt.Errorf("timeout waiting for container %s to start", containerName)
				default:
					info, err := m.dockerCli.ContainerInspect(ctx, containerName)
					if err != nil {
						return fmt.Errorf("failed to inspect container: %w", err)
					}
					if info.State.Running {
						log.Printf("Container %s started successfully", containerName)
						return nil
					}
					if info.State.ExitCode != 0 {
						return fmt.Errorf("container %s failed to start (exit code: %d)", containerName, info.State.ExitCode)
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	}
}

// RestoreBackup restores a backup of the specified service
func (m *Manager) RestoreBackup(serviceName, backupName string) error {
	service, ok := m.config.Services[serviceName]
	if !ok {
		return fmt.Errorf("service %s not found in configuration", serviceName)
	}

	// Create temporary directory for the restore
	tmpDir := filepath.Join(m.backupRoot, fmt.Sprintf("%s-restore-%d", serviceName, time.Now().Unix()))
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download the backup file
	encryptedPath := filepath.Join(tmpDir, backupName)

	// Try to download from Synology first
	err := m.Synology.Download(backupName, encryptedPath)
	if err != nil {
		// If not found in Synology and S3 is configured, try S3
		if m.S3 != nil {
			if err := m.S3.Download(backupName, encryptedPath); err != nil {
				return fmt.Errorf("failed to download backup from any storage: %w", err)
			}
		} else {
			return fmt.Errorf("failed to download backup from Synology: %w", err)
		}
	}

	// Read the encrypted backup
	encrypted, err := os.ReadFile(encryptedPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	// Decrypt the backup
	decrypted, err := crypto.Decrypt(m.key, encrypted)
	if err != nil {
		return fmt.Errorf("failed to decrypt backup: %w", err)
	}

	// Handle Docker container if specified
	if service.Docker != nil {
		if err := m.handleDockerContainer(service.Docker.Container, true); err != nil {
			return fmt.Errorf("failed to handle Docker container: %w", err)
		}
		defer m.handleDockerContainer(service.Docker.Container, false)
	}

	// Extract the archive
	if err := m.extractArchive(bytes.NewReader(decrypted), service.Path); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return nil
}

func (m *Manager) extractArchive(input io.Reader, destPath string) error {
	// Create zstd reader
	zr, err := zstd.NewReader(input)
	if err != nil {
		return fmt.Errorf("failed to create zstd reader: %w", err)
	}
	defer zr.Close()

	// Create tar reader
	tr := tar.NewReader(zr)

	// Extract each file
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Get the target path
		target := filepath.Join(destPath, header.Name)

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory with original mode
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			// If file exists and is read-only, try to make it writable
			if info, err := os.Stat(target); err == nil {
				if info.Mode()&0200 == 0 { // Check if file is read-only
					if err := os.Chmod(target, info.Mode()|0200); err != nil {
						return fmt.Errorf("failed to make file writable: %w", err)
					}
				}
			}

			// Create or overwrite file
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			// Copy contents
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file contents: %w", err)
			}
			file.Close()

		case tar.TypeSymlink:
			// Remove existing symlink if it exists
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing symlink: %w", err)
			}
			// Create new symlink
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}

		case tar.TypeLink:
			// Remove existing hard link if it exists
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing link: %w", err)
			}
			// Create new hard link
			if err := os.Link(filepath.Join(destPath, header.Linkname), target); err != nil {
				return fmt.Errorf("failed to create hard link: %w", err)
			}

		case tar.TypeChar:
			// Skip character devices as they require special privileges
			continue

		case tar.TypeBlock:
			// Skip block devices as they require special privileges
			continue

		case tar.TypeFifo:
			// Create named pipe (FIFO)
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing fifo: %w", err)
			}
			if err := unix.Mkfifo(target, uint32(header.Mode)); err != nil {
				return fmt.Errorf("failed to create fifo: %w", err)
			}

		default:
			return fmt.Errorf("unsupported file type: %d in %s", header.Typeflag, header.Name)
		}
	}

	return nil
}

// GetServices returns the configured services
func (m *Manager) GetServices() map[string]config.Service {
	return m.config.Services
}

// CleanupBackups removes old backups while keeping the most recent ones
func (m *Manager) CleanupBackups(serviceName string) (map[string]int, error) {
	deletedCounts := make(map[string]int)

	// Get services to clean up
	services := m.config.Services
	if serviceName != "" {
		service, ok := services[serviceName]
		if !ok {
			return nil, fmt.Errorf("service %s not found", serviceName)
		}
		services = map[string]config.Service{serviceName: service}
	}

	// Clean up each service
	for name, service := range services {
		// Get retain count (service-specific or global default)
		retainCount := m.config.Backup.RetainBackups
		if service.RetainBackups != nil {
			retainCount = *service.RetainBackups
		}

		// Clean up Synology backups
		synologyBackups, err := m.Synology.List(name + "-")
		if err != nil {
			return nil, fmt.Errorf("failed to list Synology backups: %w", err)
		}

		// Sort backups by modification time (newest first)
		sort.Slice(synologyBackups, func(i, j int) bool {
			timeI := parseBackupTime(synologyBackups[i].ModTime)
			timeJ := parseBackupTime(synologyBackups[j].ModTime)
			return timeI.After(timeJ)
		})

		// Keep only the most recent backups in Synology
		if len(synologyBackups) > retainCount {
			deletedCount := 0
			for _, backup := range synologyBackups[retainCount:] {
				if err := m.Synology.Delete(backup.Name); err != nil {
					// If the file doesn't exist, that's fine - it might have been deleted already
					if strings.Contains(err.Error(), "file does not exist") {
						debugLog("Skipping deletion of %s as it no longer exists", backup.Name)
						continue
					}
					return nil, fmt.Errorf("failed to delete Synology backup %s: %w", backup.Name, err)
				}
				deletedCount++
			}
			deletedCounts[name+"_synology"] = deletedCount
		}

		// Clean up S3 backups if configured
		if m.S3 != nil {
			s3Backups, err := m.S3.List(name + "-")
			if err != nil {
				return nil, fmt.Errorf("failed to list S3 backups: %w", err)
			}

			// Sort backups by modification time (newest first)
			sort.Slice(s3Backups, func(i, j int) bool {
				timeI := parseBackupTime(s3Backups[i].ModTime)
				timeJ := parseBackupTime(s3Backups[j].ModTime)
				return timeI.After(timeJ)
			})

			// Keep only the most recent backups in S3
			if len(s3Backups) > retainCount {
				deletedCount := 0
				for _, backup := range s3Backups[retainCount:] {
					if err := m.S3.Delete(backup.Name); err != nil {
						return nil, fmt.Errorf("failed to delete S3 backup %s: %w", backup.Name, err)
					}
					deletedCount++
				}
				deletedCounts[name+"_s3"] = deletedCount
			}
		}
	}

	return deletedCounts, nil
}

func parseBackupTime(timeStr string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05 UTC", timeStr)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *config.Config {
	return m.config
}

// ValidateDockerContainer checks if a Docker container exists and is accessible
func (m *Manager) ValidateDockerContainer(containerName string) error {
	ctx := context.Background()
	_, err := m.dockerCli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("failed to inspect container %s: %w", containerName, err)
	}
	return nil
}

// ValidateSynologyConnection tests the connection to the Synology NAS
func (m *Manager) ValidateSynologyConnection() error {
	// Try to list files to verify connection
	_, err := m.Synology.List("")
	if err != nil {
		return fmt.Errorf("failed to connect to Synology: %w", err)
	}
	return nil
}
