// Package security provides security support.
package security

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/security/keystore"
	"github.com/ardanlabs/kronk/sdk/security/rate"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

var (
	localFolder = "keys"
	masterFile  = "master"
)

// Config represents the config needed to constuct the security API.
type Config struct {
	OverrideBaseKeysFolder string
	Issuer                 string
}

// Security provides security support APIs.
type Security struct {
	auth    *auth.Auth
	limiter *rate.Limiter
	cfg     Config
	ks      *keystore.KeyStore
}

// New constructs a Security API.
func New(cfg Config) (*Security, error) {
	ks := keystore.New()

	a, err := auth.New(auth.Config{
		KeyLookup: ks,
		Issuer:    cfg.Issuer,
	})

	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	// -------------------------------------------------------------------------

	basePath := defaults.BaseDir(cfg.OverrideBaseKeysFolder)
	dbPath := filepath.Join(basePath, "badger")

	limiter, err := rate.New(rate.Config{
		DBPath: dbPath,
	})

	if err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	// -------------------------------------------------------------------------

	sec := Security{
		auth:    a,
		limiter: limiter,
		cfg:     cfg,
		ks:      ks,
	}

	if err := sec.addSystemKeys(); err != nil {
		return nil, fmt.Errorf("add-system-keys: %w", err)
	}

	return &sec, nil
}

// Close shutdown the security system.
func (sec *Security) Close() error {
	return sec.limiter.Close()
}

// BaseKeysFolder returns the location of the base keys folder being used.
func (sec *Security) BaseKeysFolder() string {
	return sec.cfg.OverrideBaseKeysFolder
}

// Authenticate tests the token against the requirements.
func (sec *Security) Authenticate(ctx context.Context, bearerToken string, admin bool, endpoint string) (auth.Claims, error) {
	claims, err := sec.auth.Authenticate(ctx, bearerToken)
	if err != nil {
		return auth.Claims{}, fmt.Errorf("invalid token: %w", err)
	}

	err = sec.auth.Authorize(ctx, claims, admin, endpoint)
	if err != nil {
		if errors.Is(err, auth.ErrForbidden) {
			return auth.Claims{}, fmt.Errorf("not authorized: %w", err)
		}

		return auth.Claims{}, fmt.Errorf("authorization failed: %w", err)
	}

	if claims.Admin {
		return claims, nil
	}

	limit := claims.Endpoints[endpoint]

	if err := sec.limiter.Check(claims.Subject, endpoint, limit); err != nil {
		if errors.Is(err, rate.ErrRateLimitExceeded) {
			return auth.Claims{}, fmt.Errorf("rate limit exceeded: %w", err)
		}

		return auth.Claims{}, fmt.Errorf("rate limit check failed: %w", err)
	}

	return claims, nil
}

// GenerateToken generates a new token with the specified claims.
func (sec *Security) GenerateToken(admin bool, endpoints map[string]auth.RateLimit, duration time.Duration) (string, error) {
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sec.cfg.Issuer,
			Subject:   uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
		Admin:     admin,
		Endpoints: endpoints,
	}

	token, err := sec.auth.GenerateToken(claims)
	if err != nil {
		return "", fmt.Errorf("generate-token: %w", err)
	}

	return token, nil
}

// ListKeys returns the set of keys that currently exist.
func (sec *Security) ListKeys() ([]Key, error) {
	basePath := defaults.BaseDir(sec.cfg.OverrideBaseKeysFolder)
	keysPath := filepath.Join(basePath, localFolder)

	entries, err := os.ReadDir(keysPath)
	if err != nil {
		return nil, fmt.Errorf("read-dir: %w", err)
	}

	var keys []Key

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) != ".pem" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("file-info: %w", err)
		}

		key := Key{
			ID:      entry.Name()[:len(entry.Name())-4],
			Created: info.ModTime(),
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// AddPrivateKey adds a new private key to the system. You can override the
// default location of the keys folder by passing a non-empty string.
func (sec *Security) AddPrivateKey() error {
	basePath := defaults.BaseDir(sec.cfg.OverrideBaseKeysFolder)
	keysPath := filepath.Join(basePath, localFolder)

	if err := generatePrivateKey(keysPath, uuid.NewString()); err != nil {
		return fmt.Errorf("generate-private-key: %w", err)
	}

	if _, err := sec.ks.LoadByFileSystem(os.DirFS(keysPath)); err != nil {
		return fmt.Errorf("load-by-file-system: %w", err)
	}

	return nil
}

// DeletePrivateKey removes a key from the system. Once this happens no tokens
// created with this key will authenticate.
func (sec *Security) DeletePrivateKey(keyID string) error {
	basePath := defaults.BaseDir(sec.cfg.OverrideBaseKeysFolder)
	keysPath := filepath.Join(basePath, localFolder)
	keyFile := filepath.Join(keysPath, fmt.Sprintf("%s.pem", keyID))

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return fmt.Errorf("key %q does not exist", keyID)
	}

	if err := os.Remove(keyFile); err != nil {
		return fmt.Errorf("delete-key: %w", err)
	}

	if _, err := sec.ks.LoadByFileSystem(os.DirFS(keysPath)); err != nil {
		return fmt.Errorf("load-by-file-system: %w", err)
	}

	return nil
}

// =============================================================================

func (sec *Security) addSystemKeys() error {
	basePath := defaults.BaseDir(sec.cfg.OverrideBaseKeysFolder)
	keysPath := filepath.Join(basePath, localFolder)

	os.MkdirAll(keysPath, 0755)

	n, err := sec.ks.LoadByFileSystem(os.DirFS(keysPath))
	if err != nil {
		return fmt.Errorf("load-by-file-system: %w", err)
	}

	// If the keys already exist, we are done.
	if n > 0 {
		return nil
	}

	if err := generatePrivateKey(keysPath, masterFile); err != nil {
		return fmt.Errorf("generate-private-key: %w", err)
	}

	if _, err := sec.ks.LoadByFileSystem(os.DirFS(keysPath)); err != nil {
		return fmt.Errorf("load-by-file-system: %w", err)
	}

	if err := sec.generateAdminToken(keysPath); err != nil {
		return fmt.Errorf("generate-admin-token: %w", err)
	}

	if err := generatePrivateKey(keysPath, uuid.NewString()); err != nil {
		return fmt.Errorf("generate-private-key: %w", err)
	}

	if _, err := sec.ks.LoadByFileSystem(os.DirFS(keysPath)); err != nil {
		return fmt.Errorf("load-by-file-system: %w", err)
	}

	return nil
}

func (sec *Security) generateAdminToken(keysPath string) error {
	const admin = true

	endpoints := map[string]auth.RateLimit{
		"chat-completions": {Limit: 0, Window: auth.RateUnlimited},
		"embeddings":       {Limit: 0, Window: auth.RateUnlimited},
	}

	const tenYears = time.Minute * 526000

	token, err := sec.GenerateToken(admin, endpoints, tenYears)
	if err != nil {
		return fmt.Errorf("generate admin token: %w", err)
	}

	fileName := filepath.Join(keysPath, fmt.Sprintf("%s.jwt", masterFile))

	if err := os.WriteFile(fileName, []byte(token), 0600); err != nil {
		return fmt.Errorf("write superuser token: %w", err)
	}

	return nil
}
