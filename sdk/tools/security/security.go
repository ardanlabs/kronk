// Package security provides security support.
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/security/keystore"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

var localFolder = "keys"

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
func New(log *logger.Logger, cfg Config) (*Security, error) {
	ks := keystore.New()

	basePath := defaults.BaseDir(cfg.KeysFolder)
	keysPath := filepath.Join(basePath, localFolder)

	os.MkdirAll(keysPath, 0755)

	n, err := ks.LoadByFileSystem(os.DirFS(keysPath))
	if err != nil {
		return nil, fmt.Errorf("load-by-file-system: %w", err)
	}

	// There are no keys in the system yet, create the master key.
	if n == 0 {
		if err := generateNewPrivateKey(ks, keysPath, "master"); err != nil {
			return nil, fmt.Errorf("generate-new-key: %w", err)
		}
	}

	authCfg := auth.Config{
		KeyLookup: ks,
		Issuer:    cfg.Issuer,
		Enabled:   cfg.Enabled,
	}

	sec := Security{
		Auth: auth.New(authCfg),
		cfg:  cfg,
		ks:   ks,
	}

	// We need an admin token and a new private key for moving forward with
	// creating user tokens.
	if n == 0 {
		if err := sec.generateAdminToken(keysPath); err != nil {
			return nil, fmt.Errorf("generate-super-user-token: %w", err)
		}

		if err := generateNewPrivateKey(ks, keysPath, uuid.NewString()); err != nil {
			return nil, fmt.Errorf("generate-new-key: %w", err)
		}
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

func generateNewPrivateKey(ks *keystore.KeyStore, keysPath string, keyName string) error {
	if err := generatePrivateKey(keysPath, keyName); err != nil {
		return fmt.Errorf("generate-private-key: %w", err)
	}

	if _, err := ks.LoadByFileSystem(os.DirFS(keysPath)); err != nil {
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

	fileName := filepath.Join(keysPath, "admin-read-and-delete.jwt")

	if err := os.WriteFile(fileName, []byte(token), 0600); err != nil {
		return fmt.Errorf("write superuser token: %w", err)
	}

	return nil
}
