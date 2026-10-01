package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func generatePrivateKey(keysPath string, keyName string) error {

	// Generate a new private key.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate-private-key: unable to generate key: %w", err)
	}

	fileName := fmt.Sprintf("%s.pem", keyName)
	keyName = filepath.Join(keysPath, fileName)

	// Create a sibling temporary file so an interrupted write never publishes
	// a truncated key that prevents the keystore from loading.
	privateFile, err := os.CreateTemp(keysPath, "."+fileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary private file: %w", err)
	}
	tmpName := privateFile.Name()
	defer os.Remove(tmpName)

	if err := privateFile.Chmod(0600); err != nil {
		return errors.Join(fmt.Errorf("setting private file permissions: %w", err), privateFile.Close())
	}

	// Construct a PEM block for the private key.
	privateBlock := pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	// Write the private key to the private key file.
	if err := pem.Encode(privateFile, &privateBlock); err != nil {
		return errors.Join(fmt.Errorf("generate-private-key: unable to encode: %w", err), privateFile.Close())
	}
	if err := privateFile.Sync(); err != nil {
		return errors.Join(fmt.Errorf("generate-private-key: unable to sync: %w", err), privateFile.Close())
	}
	if err := privateFile.Close(); err != nil {
		return fmt.Errorf("generate-private-key: unable to close: %w", err)
	}
	if err := os.Rename(tmpName, keyName); err != nil {
		return fmt.Errorf("generate-private-key: unable to publish: %w", err)
	}

	return nil
}
