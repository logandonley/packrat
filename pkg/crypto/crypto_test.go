package crypto

import (
	"bytes"
	"testing"
)

// TestDeterministicKeyDerivation tests that the key derivation process is deterministic
func TestDeterministicKeyDerivation(t *testing.T) {
	// Test that same password produces same salt
	password1 := "test-password"
	salt1 := generateDeterministicSalt(password1)
	salt2 := generateDeterministicSalt(password1)
	if !bytes.Equal(salt1, salt2) {
		t.Error("Same password produced different salts")
	}

	// Test that same password and salt produce same key
	key1, salt1, _ := DeriveKey(password1)
	key2, salt2, _ := DeriveKey(password1)
	if !bytes.Equal(key1, key2) {
		t.Error("Same password produced different keys")
	}

	// Test that different passwords produce different salts and keys
	password2 := "different-password"
	key3, salt3, _ := DeriveKey(password2)
	if bytes.Equal(salt1, salt3) {
		t.Error("Different passwords produced same salt")
	}
	if bytes.Equal(key1, key3) {
		t.Error("Different passwords produced same key")
	}

	// Test that RecreateKey produces same key with same password and salt
	recreatedKey := RecreateKey(password1, salt1)
	if !bytes.Equal(key1, recreatedKey) {
		t.Error("RecreateKey produced different key")
	}
}
