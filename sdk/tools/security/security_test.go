package security_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/security"
)

func TestGenerateToken(t *testing.T) {
	tmpDir := t.TempDir()

	sec, err := security.New(security.Config{
		OverrideBaseKeysFolder: tmpDir,
		Issuer:                 "test-issuer",
	})
	if err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	endpoints := []string{"chat-completions"}

	token, err := sec.GenerateToken("test-subject", true, endpoints, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestAddPrivateKey(t *testing.T) {
	tmpDir := t.TempDir()

	sec, err := security.New(security.Config{
		OverrideBaseKeysFolder: tmpDir,
		Issuer:                 "test-issuer",
	})

	if err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	initialKeys := countKeys(t, filepath.Join(tmpDir, "keys"))

	if err := sec.AddPrivateKey(); err != nil {
		t.Fatalf("failed to add private key: %v", err)
	}

	afterKeys := countKeys(t, filepath.Join(tmpDir, "keys"))

	if afterKeys != initialKeys+1 {
		t.Errorf("expected %d keys, got %d", initialKeys+1, afterKeys)
	}
}

func TestDeletePrivateKey(t *testing.T) {
	tmpDir := t.TempDir()

	sec, err := security.New(security.Config{
		OverrideBaseKeysFolder: tmpDir,
		Issuer:                 "test-issuer",
	})

	if err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	if err := sec.AddPrivateKey(); err != nil {
		t.Fatalf("failed to add private key: %v", err)
	}

	keysPath := filepath.Join(tmpDir, "keys")
	keyID := findNonMasterKey(t, keysPath)

	if err := sec.DeletePrivateKey(keyID); err != nil {
		t.Fatalf("failed to delete private key: %v", err)
	}

	keyFile := filepath.Join(keysPath, keyID+".pem")
	if _, err := os.Stat(keyFile); !os.IsNotExist(err) {
		t.Errorf("expected key file to be deleted")
	}
}

func TestDeletePrivateKey_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	sec, err := security.New(security.Config{
		OverrideBaseKeysFolder: tmpDir,
		Issuer:                 "test-issuer",
	})

	if err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	err = sec.DeletePrivateKey("non-existent-key")
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
}

// =============================================================================

func countKeys(t *testing.T, keysPath string) int {
	t.Helper()

	entries, err := os.ReadDir(keysPath)
	if err != nil {
		t.Fatalf("failed to read keys directory: %v", err)
	}

	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".pem" {
			count++
		}
	}

	return count
}

func findNonMasterKey(t *testing.T, keysPath string) string {
	t.Helper()

	entries, err := os.ReadDir(keysPath)
	if err != nil {
		t.Fatalf("failed to read keys directory: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".pem" && name != "master.pem" {
			return name[:len(name)-4]
		}
	}

	t.Fatal("no non-master key found")

	return ""
}
