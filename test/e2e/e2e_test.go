package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/logandonley/packrat/pkg/backup"
	"github.com/logandonley/packrat/pkg/config"
	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/logandonley/packrat/pkg/daemon"
)

const testConfig = `encryption:
  key_file: %s

services:
  test-service:
    path: %s
    schedule: "* * * * *"
    docker:
      container: packrat-test

backup:
  retain_backups: 5
  synology:
    host: %s
    port: %d
    username: %s
    key_file: %s
    path: ./backups/test/packrat-e2e
`

// cleanupBackups removes all backups for a service
func cleanupBackups(t *testing.T, manager *backup.Manager, serviceName string) {
	t.Helper()
	backups, err := manager.Synology.List(serviceName)
	if err != nil {
		t.Logf("Warning: Failed to list backups during cleanup: %v", err)
		return
	}
	for _, b := range backups {
		if err := manager.Synology.Delete(b.Name); err != nil {
			t.Logf("Warning: Failed to delete backup %s during cleanup: %v", b.Name, err)
		}
	}
}

func TestE2E(t *testing.T) {
	// Create temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "packrat-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test data
	testDataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		t.Fatalf("Failed to create test data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDataDir, "test.txt"), []byte("test data"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create encryption key
	keyPath := filepath.Join(tmpDir, "key")
	key, salt, err := crypto.DeriveKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	if err := crypto.SaveKey(key, salt, keyPath); err != nil {
		t.Fatalf("Failed to save key: %v", err)
	}

	// Create config file
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	// Get Synology config from environment variables
	synologyHost := os.Getenv("PACKRAT_TEST_SYNOLOGY_HOST")
	if synologyHost == "" {
		t.Skip("PACKRAT_TEST_SYNOLOGY_HOST not set")
	}
	synologyPort := 22
	synologyUser := os.Getenv("PACKRAT_TEST_SYNOLOGY_USER")
	if synologyUser == "" {
		t.Skip("PACKRAT_TEST_SYNOLOGY_USER not set")
	}
	synologyKeyFile := os.Getenv("PACKRAT_TEST_SYNOLOGY_KEY_FILE")
	if synologyKeyFile == "" {
		t.Skip("PACKRAT_TEST_SYNOLOGY_KEY_FILE not set")
	}

	// Write config file
	configContent := fmt.Sprintf(testConfig,
		keyPath,
		testDataDir,
		synologyHost,
		synologyPort,
		synologyUser,
		synologyKeyFile,
	)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Pull nginx image for testing
	cmd := exec.Command("docker", "pull", "nginx:alpine")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to pull nginx image: %v", err)
	}

	// Create and start container
	cmd = exec.Command("docker", "run", "-d", "--name", "packrat-test", "nginx:alpine")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	defer exec.Command("docker", "rm", "-f", "packrat-test").Run()

	// Load config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create backup manager
	manager, err := backup.NewManager(cfg, key)
	if err != nil {
		t.Fatalf("Failed to create backup manager: %v", err)
	}
	defer manager.Close()
	defer cleanupBackups(t, manager, "test-service") // Always clean up backups

	// Clean up any existing backups before starting
	cleanupBackups(t, manager, "test-service")

	// Test manual backup
	t.Log("Testing manual backup...")
	if err := manager.CreateBackup("test-service"); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// List backups to verify
	backups, err := manager.Synology.List("test-service")
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup, got %d", len(backups))
	}

	// Test restore
	t.Log("Testing restore...")
	restoreDir := filepath.Join(tmpDir, "restore")
	if err := os.MkdirAll(restoreDir, 0755); err != nil {
		t.Fatalf("Failed to create restore dir: %v", err)
	}

	// Modify original file to verify restore
	if err := os.WriteFile(filepath.Join(testDataDir, "test.txt"), []byte("modified data"), 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Restore backup
	if err := manager.RestoreBackup("test-service", backups[0].Name); err != nil {
		t.Fatalf("Failed to restore backup: %v", err)
	}

	// Verify restored data
	data, err := os.ReadFile(filepath.Join(testDataDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(data) != "test data" {
		t.Fatalf("Restored data does not match: got %q, want %q", string(data), "test data")
	}

	// Test daemon
	t.Log("Testing daemon...")
	d := daemon.New(cfg, manager)

	// Start daemon in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for a backup to be created by the daemon
	t.Log("Waiting for scheduled backup...")
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		backups, err := manager.Synology.List("test-service")
		if err != nil {
			t.Fatalf("Failed to list backups: %v", err)
		}
		if len(backups) > 1 {
			t.Log("Daemon created a backup successfully")
			break
		}
		time.Sleep(5 * time.Second)
	}

	// Stop daemon
	d.Stop()
	if err := <-errCh; err != nil {
		t.Fatalf("Daemon error: %v", err)
	}

	// Test cleanup by setting retain_backups to 0
	t.Log("Testing backup cleanup...")
	cfg.Backup.RetainBackups = 0
	deletedCounts, err := manager.CleanupBackups("test-service")
	if err != nil {
		t.Fatalf("Failed to cleanup backups: %v", err)
	}
	if count := deletedCounts["test-service"]; count == 0 {
		t.Fatal("Expected some backups to be cleaned up, but none were deleted")
	}

	// Verify all backups were deleted
	backups, err = manager.Synology.List("test-service")
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("Expected 0 backups after cleanup, got %d", len(backups))
	}

	t.Log("E2E test completed successfully")
}
