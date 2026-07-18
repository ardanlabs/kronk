package kronk

import (
	"encoding/hex"
	"errors"
)

func validateAdminConfig(adminAuth, webAdmin bool, passwordSHA256, authHost string) error {
	if passwordSHA256 != "" && !adminAuth {
		return errors.New("configuration: web admin password requires admin authentication")
	}
	if passwordSHA256 != "" {
		decoded, err := hex.DecodeString(passwordSHA256)
		if err != nil || len(decoded) != 32 {
			return errors.New("configuration: web admin password SHA-256 must be exactly 64 hexadecimal characters")
		}
	}
	if webAdmin && adminAuth && passwordSHA256 == "" {
		return errors.New("configuration: protected web admin requires a password SHA-256")
	}
	if webAdmin && passwordSHA256 != "" && authHost != "" {
		return errors.New("configuration: web password login does not support an external auth host")
	}

	return nil
}
