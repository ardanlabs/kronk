// Package security provides security support.
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/security/keystore"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

var (
	localFolder = "keys"
	masterFile  = "master"
)

// Config represents the config needed to constuct the security API.
type Config struct {
	KeysFolder string
	Issuer     string
	Enabled    bool
}

// Security provides security support APIs.
type Security struct {
	Auth *auth.Auth
	cfg  Config
	ks   *keystore.KeyStore
}

// New constructs a Security API.
func New(cfg Config) (*Security, error) {
	ks := keystore.New()

	a, err := auth.New(auth.Config{
		KeyLookup: ks,
		Issuer:    cfg.Issuer,
		Enabled:   cfg.Enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	sec := Security{
		Auth: a,
		cfg:  cfg,
		ks:   ks,
	}

	if err := sec.addSystemKeys(cfg.KeysFolder); err != nil {
		return nil, fmt.Errorf("add-system-keys: %w", err)
	}

	return &sec, nil
}

// GenerateToken generates a new token with the specified claims.
func (sec *Security) GenerateToken(subject string, admin bool, endpoints map[string]bool, duration time.Duration) (string, error) {
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sec.cfg.Issuer,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
		Admin:     admin,
		Endpoints: endpoints,
	}

	token, err := sec.Auth.GenerateToken(claims)
	if err != nil {
		return "", fmt.Errorf("generate-token: %w", err)
	}

	return token, nil
}

// =============================================================================

func (sec *Security) addSystemKeys(keysFolder string) error {
	basePath := defaults.BaseDir(keysFolder)
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

	endpoints := map[string]bool{
		"chat-completions": true,
		"embeddings":       true,
	}

	const tenYears = time.Minute * 526000

	token, err := sec.GenerateToken("admin", admin, endpoints, tenYears)
	if err != nil {
		return fmt.Errorf("generate admin token: %w", err)
	}

	fileName := filepath.Join(keysPath, fmt.Sprintf("%s.jwt", masterFile))

	if err := os.WriteFile(fileName, []byte(token), 0600); err != nil {
		return fmt.Errorf("write superuser token: %w", err)
	}

	return nil
}
