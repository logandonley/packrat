package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

const (
	// KeySize is the size of the encryption key in bytes (32 bytes = 256 bits)
	KeySize = 32
	// SaltSize is the size of the salt used in key derivation
	SaltSize = 16
	// Memory in KiB used by Argon2
	Memory = 64 * 1024
	// Iterations used by Argon2
	Iterations = 3
	// Parallelism used by Argon2
	Parallelism = 2
)

// generateDeterministicSalt generates a deterministic salt from a password using SHA-256
func generateDeterministicSalt(password string) []byte {
	// Use SHA-256 to generate a deterministic hash from the password
	hasher := sha256.New()
	hasher.Write([]byte(password))
	hash := hasher.Sum(nil)

	// Take the first SaltSize bytes as our salt
	salt := make([]byte, SaltSize)
	copy(salt, hash[:SaltSize])
	return salt
}

// DeriveKey derives an encryption key from a password using Argon2
func DeriveKey(password string) ([]byte, []byte, error) {
	// Generate deterministic salt from password
	salt := generateDeterministicSalt(password)

	key := argon2.IDKey([]byte(password), salt, Iterations, Memory, Parallelism, KeySize)
	return key, salt, nil
}

// RecreateKey recreates an encryption key from a password and salt
func RecreateKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, Iterations, Memory, Parallelism, KeySize)
}

// SaveKey saves the encryption key to a file
func SaveKey(key, salt []byte, keyPath string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Save key and salt as hex strings
	content := fmt.Sprintf("%x\n%x", key, salt)
	return os.WriteFile(keyPath, []byte(content), 0600)
}

// LoadKey loads the encryption key from a file
func LoadKey(keyPath string) ([]byte, []byte, error) {
	content, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var key, salt []byte
	_, err = fmt.Sscanf(string(content), "%x\n%x", &key, &salt)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse key file: %w", err)
	}

	return key, salt, nil
}

// Encrypt encrypts data using AES-256-GCM
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-256-GCM
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// GenerateAndSaveKey generates a new encryption key from a password and saves it to a file
func GenerateAndSaveKey(password []byte, path string) error {
	// Generate key and salt
	key, salt, err := DeriveKey(string(password))
	if err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}

	// Save key and salt
	if err := SaveKey(key, salt, path); err != nil {
		return fmt.Errorf("failed to save key: %w", err)
	}

	return nil
}
