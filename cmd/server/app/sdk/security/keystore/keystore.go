// Package keystore implements the auth.KeyLookup interface. This implements
// an in-memory keystore for JWT support.
package keystore

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"
)

// ErrKeyNotFound is returned when a key identified by a kid is not found.
var ErrKeyNotFound = errors.New("key not found")

const maxPEMFileSize = 1024 * 1024 // 1 MB

// Key represents Key information.
type Key struct {
	keyID      string
	privatePEM string
	publicPEM  string
}

// KeyStore represents an in memory store implementation of the
// KeyLookup interface for use with the auth package.
type KeyStore struct {
	activeKey Key
	store     map[string]Key
	mu        sync.RWMutex
}

// New constructs an empty KeyStore ready for use.
func New() *KeyStore {
	return &KeyStore{
		store: make(map[string]Key),
	}
}

// LoadByFileSystem loads a set of RSA PEM files rooted inside of a directory. The
// name of each PEM file will be used as the key id. The youngest file by
// modification time becomes the active key. The function also returns the total
// number of keys in the store.
// Example: ks.LoadByFileSystem(os.DirFS("/zarf/keys/"))
// Example: /zarf/keys/54bb2165-71e1-41a6-af3e-7da4a0e1e2c1.pem
func (ks *KeyStore) LoadByFileSystem(fsys fs.FS) (int, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	store := make(map[string]Key)

	var youngestKey Key
	var youngestTime time.Time

	fn := func(fileName string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walkdir failure: %w", err)
		}

		if dirEntry.IsDir() {
			return nil
		}

		if path.Ext(fileName) != ".pem" {
			return nil
		}

		file, err := fsys.Open(fileName)
		if err != nil {
			return fmt.Errorf("opening key file: %w", err)
		}
		defer file.Close()

		// limit PEM file size to 1 megabyte. This should be reasonable for
		// almost any PEM file and prevents shenanigans like linking the file
		// to /dev/random or something like that.
		pem, err := io.ReadAll(io.LimitReader(file, maxPEMFileSize))
		if err != nil {
			return fmt.Errorf("reading auth private key: %w", err)
		}

		privatePEM := string(pem)
		publicPEM, err := toPublicPEM(privatePEM)
		if err != nil {
			return fmt.Errorf("converting private PEM to public: %w", err)
		}

		keyID := strings.TrimSuffix(path.Base(fileName), ".pem")

		key := Key{
			keyID:      keyID,
			privatePEM: privatePEM,
			publicPEM:  publicPEM,
		}

		info, err := dirEntry.Info()
		if err != nil {
			return fmt.Errorf("getting file info: %w", err)
		}

		if info.ModTime().After(youngestTime) {
			youngestTime = info.ModTime()
			youngestKey = key
		}

		store[keyID] = key

		return nil
	}

	if err := fs.WalkDir(fsys, ".", fn); err != nil {
		return 0, fmt.Errorf("walking directory: %w", err)
	}

	ks.activeKey = youngestKey
	ks.store = store

	return len(ks.store), nil
}

// PrivateKey returns the current active private key.
func (ks *KeyStore) PrivateKey() (string, string) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	return ks.activeKey.keyID, ks.activeKey.privatePEM
}

// PublicKey searches the key store for a given kid and returns the public key.
func (ks *KeyStore) PublicKey(kid string) (string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	key, found := ks.store[kid]
	if !found {
		return "", ErrKeyNotFound
	}

	return key.publicPEM, nil
}

func toPublicPEM(privatePEM string) (string, error) {
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return "", errors.New("invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key")
	}

	var parsedKey any
	parsedKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsedKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse private key as PKCS1 or PKCS8: %w", err)
		}
	}

	pk, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("key is not a valid RSA private key")
	}

	asn1Bytes, err := x509.MarshalPKIXPublicKey(&pk.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshaling public key: %w", err)
	}

	publicBlock := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: asn1Bytes,
	}

	var buf bytes.Buffer
	if err := pem.Encode(&buf, &publicBlock); err != nil {
		return "", fmt.Errorf("encoding to public PEM: %w", err)
	}

	return buf.String(), nil
}
