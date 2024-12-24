package backup

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/logandonley/packrat/pkg/config"
	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/logandonley/packrat/pkg/storage"
)

// mockStorage implements storage.Storage for testing
type mockStorage struct {
	files map[string][]byte
}

func (m *mockStorage) Upload(localPath, remoteName string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	m.files[remoteName] = data
	return nil
}

func (m *mockStorage) Download(remoteName, localPath string) error {
	data, ok := m.files[remoteName]
	if !ok {
		return os.ErrNotExist
	}
	return os.WriteFile(localPath, data, 0600)
}

func (m *mockStorage) List(prefix string) ([]storage.BackupFile, error) {
	var files []storage.BackupFile
	for name, data := range m.files {
		if prefix == "" || name[:len(prefix)] == prefix {
			files = append(files, storage.BackupFile{
				Name:    name,
				Size:    int64(len(data)),
				ModTime: time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
			})
		}
	}
	return files, nil
}

func (m *mockStorage) Close() error {
	return nil
}

func (m *mockStorage) Delete(remoteName string) error {
	delete(m.files, remoteName)
	return nil
}

// MockStorage implements storage.Storage for testing
type MockStorage struct {
	files     map[string]storage.BackupFile
	deleted   []string
	listErr   error
	deleteErr error
}

func (m *MockStorage) Upload(localPath, remoteName string) error {
	return nil
}

func (m *MockStorage) Download(remoteName, localPath string) error {
	return nil
}

func (m *MockStorage) List(prefix string) ([]storage.BackupFile, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var result []storage.BackupFile
	for name, file := range m.files {
		if prefix == "" || strings.HasPrefix(name, prefix) {
			result = append(result, file)
		}
	}
	return result, nil
}

func (m *MockStorage) Delete(remoteName string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleted = append(m.deleted, remoteName)
	delete(m.files, remoteName)
	return nil
}

func (m *MockStorage) Close() error {
	return nil
}

func TestBackupManager_CreateBackup(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "backup-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test data
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create test config
	cfg := &config.Config{
		Services: map[string]config.Service{
			"test": {
				Path: tmpDir,
			},
		},
	}

	// Create encryption key
	key := []byte("testkey0123456789012345678901234")

	// Create mock storage
	mockStorage := &mockStorage{
		files: make(map[string][]byte),
	}

	// Create backup manager
	manager := &Manager{
		config:     cfg,
		key:        key,
		backupRoot: tmpDir,
		Synology:   mockStorage,
	}

	// Create backup
	if err := manager.CreateBackup("test"); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup was created
	files, err := mockStorage.List("")
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 backup, got %d", len(files))
	}

	// Verify backup contents
	for name, data := range mockStorage.files {
		t.Logf("Found backup: %s", name)
		decrypted, err := crypto.Decrypt(key, data)
		if err != nil {
			t.Errorf("Failed to decrypt backup: %v", err)
		}
		t.Logf("Decrypted backup size: %d bytes", len(decrypted))
	}
}

func TestManager_RestoreBackup(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := filepath.Join(os.TempDir(), "packrat-test")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test data
	testData := []byte("test data")
	compressed := &bytes.Buffer{}
	zw, err := zstd.NewWriter(compressed)
	if err != nil {
		t.Fatalf("Failed to create zstd writer: %v", err)
	}
	tw := tar.NewWriter(zw)

	// Add a file to the archive
	hdr := &tar.Header{
		Name: "test.txt",
		Mode: 0600,
		Size: int64(len(testData)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tw.Write(testData); err != nil {
		t.Fatalf("Failed to write tar data: %v", err)
	}
	tw.Close()
	zw.Close()

	// Create test key (32 bytes for AES-256)
	key := []byte("testkey0123456789012345678901234")

	// Encrypt the compressed data
	encrypted, err := crypto.Encrypt(key, compressed.Bytes())
	if err != nil {
		t.Fatalf("Failed to encrypt data: %v", err)
	}

	// Create mock storage
	mockStorage := &mockStorage{
		files: map[string][]byte{
			"test-backup.tar.gz": encrypted,
		},
	}

	// Create test config
	cfg := &config.Config{
		Services: map[string]config.Service{
			"test": {
				Path: filepath.Join(tmpDir, "test"),
			},
		},
	}

	// Create manager
	manager := &Manager{
		config:     cfg,
		Synology:   mockStorage,
		key:        key,
		backupRoot: tmpDir,
	}

	// Test restore
	if err := manager.RestoreBackup("test", "test-backup.tar.gz"); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restored file
	restored, err := os.ReadFile(filepath.Join(tmpDir, "test", "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if !bytes.Equal(restored, testData) {
		t.Errorf("Restored data does not match original. Got %q, want %q", restored, testData)
	}
}

func TestExcludePatterns(t *testing.T) {
	// Create a temporary test directory structure
	tmpDir, err := os.MkdirTemp("", "backup-exclude-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files and directories
	files := []string{
		"file1.txt",
		"file2.log",
		"node_modules/package1/index.js",
		"node_modules/package2/index.js",
		"src/code.js",
		"src/node_modules/local-pkg/index.js",
		".git/config",
		"dist/bundle.js",
		"build/output.js",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Create test config
	cfg := &config.Config{
		Services: map[string]config.Service{
			"test": {
				Path: tmpDir,
				Exclude: []string{
					"**/node_modules/**",
					"**/.git/**",
					"**/dist/**",
					"**/build/**",
				},
			},
		},
	}

	// Create backup manager
	manager := &Manager{
		config:     cfg,
		backupRoot: tmpDir,
	}

	// Create archive
	var buf bytes.Buffer
	if err := manager.createArchive(tmpDir, &buf); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Read the archive and check its contents
	zr, err := zstd.NewReader(&buf)
	if err != nil {
		t.Fatalf("Failed to create zstd reader: %v", err)
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	var includedFiles []string
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		includedFiles = append(includedFiles, header.Name)
	}

	// Check that excluded patterns are not in the archive
	for _, file := range includedFiles {
		if strings.Contains(file, "node_modules") {
			t.Errorf("node_modules should be excluded but found: %s", file)
		}
		if strings.Contains(file, ".git") {
			t.Errorf(".git should be excluded but found: %s", file)
		}
		if strings.Contains(file, "dist") {
			t.Errorf("dist should be excluded but found: %s", file)
		}
		if strings.Contains(file, "build") {
			t.Errorf("build should be excluded but found: %s", file)
		}
	}

	// Check that non-excluded files are in the archive
	expectedFiles := []string{
		"file1.txt",
		"file2.log",
		"src/code.js",
	}

	for _, expected := range expectedFiles {
		found := false
		for _, actual := range includedFiles {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file %s not found in archive", expected)
		}
	}

	// Print included files for debugging
	t.Logf("Files included in archive: %v", includedFiles)
}

func TestCleanupBackups(t *testing.T) {
	retain2 := 2
	retain1 := 1

	tests := []struct {
		name          string
		config        *config.Config
		existingFiles map[string]storage.BackupFile
		serviceName   string
		listErr       error
		deleteErr     error
		wantDeleted   []string
		wantErr       bool
	}{
		{
			name: "deletes old backups keeping retention count",
			config: &config.Config{
				Services: map[string]config.Service{
					"test": {
						RetainBackups: &retain2,
					},
				},
			},
			existingFiles: map[string]storage.BackupFile{
				"test-2024-01-01T00:00:00Z.enc": {Name: "test-2024-01-01T00:00:00Z.enc", ModTime: "2024-01-01 00:00:00 UTC"},
				"test-2024-01-02T00:00:00Z.enc": {Name: "test-2024-01-02T00:00:00Z.enc", ModTime: "2024-01-02 00:00:00 UTC"},
				"test-2024-01-03T00:00:00Z.enc": {Name: "test-2024-01-03T00:00:00Z.enc", ModTime: "2024-01-03 00:00:00 UTC"},
			},
			serviceName: "test",
			wantDeleted: []string{"test-2024-01-01T00:00:00Z.enc"},
		},
		{
			name: "handles multiple services",
			config: &config.Config{
				Backup: config.BackupConfiguration{
					RetainBackups: retain1,
				},
				Services: map[string]config.Service{
					"test1": {},
					"test2": {},
				},
			},
			existingFiles: map[string]storage.BackupFile{
				"test1-2024-01-01T00:00:00Z.enc": {Name: "test1-2024-01-01T00:00:00Z.enc", ModTime: "2024-01-01 00:00:00 UTC"},
				"test1-2024-01-02T00:00:00Z.enc": {Name: "test1-2024-01-02T00:00:00Z.enc", ModTime: "2024-01-02 00:00:00 UTC"},
				"test2-2024-01-01T00:00:00Z.enc": {Name: "test2-2024-01-01T00:00:00Z.enc", ModTime: "2024-01-01 00:00:00 UTC"},
				"test2-2024-01-02T00:00:00Z.enc": {Name: "test2-2024-01-02T00:00:00Z.enc", ModTime: "2024-01-02 00:00:00 UTC"},
			},
			wantDeleted: []string{
				"test1-2024-01-01T00:00:00Z.enc",
				"test2-2024-01-01T00:00:00Z.enc",
			},
		},
		{
			name: "handles list error",
			config: &config.Config{
				Services: map[string]config.Service{
					"test": {},
				},
			},
			listErr: fmt.Errorf("list error"),
			wantErr: true,
		},
		{
			name: "handles delete error",
			config: &config.Config{
				Services: map[string]config.Service{
					"test": {
						RetainBackups: &retain1,
					},
				},
			},
			existingFiles: map[string]storage.BackupFile{
				"test-2024-01-01T00:00:00Z.enc": {Name: "test-2024-01-01T00:00:00Z.enc", ModTime: "2024-01-01 00:00:00 UTC"},
				"test-2024-01-02T00:00:00Z.enc": {Name: "test-2024-01-02T00:00:00Z.enc", ModTime: "2024-01-02 00:00:00 UTC"},
			},
			deleteErr: fmt.Errorf("delete error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := &MockStorage{
				files:     tt.existingFiles,
				listErr:   tt.listErr,
				deleteErr: tt.deleteErr,
			}

			manager := &Manager{
				config:   tt.config,
				Synology: mockStorage,
			}

			_, err := manager.CleanupBackups(tt.serviceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("CleanupBackups() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Sort both slices to ensure consistent comparison
				sort.Strings(mockStorage.deleted)
				sort.Strings(tt.wantDeleted)

				if !reflect.DeepEqual(mockStorage.deleted, tt.wantDeleted) {
					t.Errorf("CleanupBackups() deleted = %v, want %v", mockStorage.deleted, tt.wantDeleted)
				}
			}
		})
	}
}
