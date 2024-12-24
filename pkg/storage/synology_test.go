package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func getTestConfig() *SynologyConfig {
	// Try environment variables first
	host := os.Getenv("PACKRAT_TEST_SYNOLOGY_HOST")
	portStr := os.Getenv("PACKRAT_TEST_SYNOLOGY_PORT")
	username := os.Getenv("PACKRAT_TEST_SYNOLOGY_USER")
	keyFile := os.Getenv("PACKRAT_TEST_SYNOLOGY_KEY")
	path := os.Getenv("PACKRAT_TEST_SYNOLOGY_PATH")

	// If any env vars are missing, use default values from local dev config
	if host == "" {
		host = "192.168.1.100"
	}
	port := 22
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	if username == "" {
		username = "backups"
	}
	if keyFile == "" {
		keyFile = "/home/backups/.ssh/id_rsa"
	}
	if path == "" {
		path = "backups/test/"
	}

	return &SynologyConfig{
		Host:     host,
		Port:     port,
		Username: username,
		KeyFile:  keyFile,
		Path:     path,
	}
}

func TestSynologyStorage_List(t *testing.T) {
	config := getTestConfig()

	// Create storage instance
	s, err := NewSynologyStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer s.Close()

	// List files
	files, err := s.List("")
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	// Just verify we can list files without error
	t.Logf("Found %d files", len(files))
}

func TestSynologyStorage_Upload(t *testing.T) {
	config := getTestConfig()

	// Create storage instance
	s, err := NewSynologyStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer s.Close()

	// Create a test file
	testDir, err := os.MkdirTemp("", "packrat-synology-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	testFile := filepath.Join(testDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Clean up any existing test file
	_ = s.sftpClient.Remove(filepath.Join(config.Path, "test-upload.txt"))

	// Upload test file
	if err := s.Upload(testFile, "test-upload.txt"); err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}

	// Verify file exists
	_, err = s.sftpClient.Stat(filepath.Join(config.Path, "test-upload.txt"))
	if err != nil {
		t.Errorf("Uploaded file not found: %v", err)
	}

	// Clean up remote file
	if err := s.sftpClient.Remove(filepath.Join(config.Path, "test-upload.txt")); err != nil {
		t.Logf("Warning: Failed to clean up remote file: %v", err)
	}
}
