package kronk

import (
	"encoding/hex"
	"errors"
	"time"
)

func validateTimeoutConfig(inferenceTimeout, writeTimeout time.Duration) error {
	if inferenceTimeout <= 0 {
		return errors.New("configuration: web inference timeout must be greater than zero")
	}
	if writeTimeout != 0 && writeTimeout <= inferenceTimeout {
		return errors.New("configuration: web write timeout must be disabled or greater than the inference timeout")
	}

	return nil
}

func validateSessionsConfig(enabled bool, maxCompleted int) error {
	if enabled && maxCompleted < 1 {
		return errors.New("configuration: sessions max completed must be greater than zero")
	}

	return nil
}

func validateAdminConfig(adminAuth, webAdmin bool, passwordSHA256, authHost string) error {
	if passwordSHA256 != "" {
		decoded, err := hex.DecodeString(passwordSHA256)
		if err != nil || len(decoded) != 32 {
			return errors.New("configuration: web admin password SHA-256 must be exactly 64 hexadecimal characters")
		}
	}
	if webAdmin && adminAuth && passwordSHA256 == "" {
		return errors.New("configuration: protected web admin requires a password SHA-256")
	}
	if webAdmin && adminAuth && passwordSHA256 != "" && authHost != "" {
		return errors.New("configuration: web password login does not support an external auth host")
	}

	return nil
}
