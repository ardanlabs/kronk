package models

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const verifiedSuffix = ".verified"

type verifiedSentinel struct {
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	MTimeNS      int64  `json:"mtime_ns"`
	VerifiedAt   int64  `json:"verified_at"`
	KronkVersion string `json:"kronk_version,omitempty"`
}

func checkModel(modelFile string, checkSHA bool) error {
	integrity, exists, err := loadArtifactIntegrity(modelFile)
	if err != nil {
		return fmt.Errorf("check-model: %w", err)
	}
	if !exists {
		return nil
	}

	verification, changed, err := model.VerifyArtifact(modelFile, integrity.Digest, integrity.Verification, checkSHA)
	if err != nil {
		return fmt.Errorf("check-model: %w", err)
	}
	if changed {
		// The verification record is only a cache. Preserve the existing
		// behavior for read-only model stores: successful artifact verification
		// must not fail merely because the cache cannot be persisted.
		_ = writeArtifactVerification(modelFile, verification)
	}

	return nil
}

func loadArtifactIntegrity(modelFile string) (model.ArtifactIntegrity, bool, error) {
	digest, err := readArtifactDigest(modelFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.ArtifactIntegrity{}, false, nil
		}
		return model.ArtifactIntegrity{}, false, err
	}

	var previous *model.ArtifactVerification
	verification, err := readArtifactVerification(modelFile)
	if err == nil {
		previous = &verification
	}

	integrity := model.ArtifactIntegrity{
		Digest:       digest,
		Verification: previous,
	}

	return integrity, true, nil
}

func configureArtifactIntegrity(cfg *model.Config) error {
	paths := make([]string, 0, len(cfg.ModelFiles)+2)
	paths = append(paths, cfg.ModelFiles...)
	if cfg.ProjFile != "" {
		paths = append(paths, cfg.ProjFile)
	}
	if cfg.MTPDrafterFile != "" {
		paths = append(paths, cfg.MTPDrafterFile)
	}
	if cfg.PtrDraftModel != nil {
		paths = append(paths, cfg.PtrDraftModel.ModelFiles...)
	}

	integrityByPath := make(map[string]model.ArtifactIntegrity, len(paths))
	for _, modelFile := range paths {
		integrity, exists, err := loadArtifactIntegrity(modelFile)
		if err != nil {
			return fmt.Errorf("configure artifact integrity for %q: %w", modelFile, err)
		}
		if exists {
			integrityByPath[modelFile] = integrity
		}
	}

	if len(integrityByPath) > 0 {
		cfg.ArtifactIntegrity = integrityByPath
		cfg.RecordArtifactVerification = writeArtifactVerification
	}

	return nil
}

func readArtifactDigest(modelFile string) (model.ArtifactDigest, error) {
	data, err := os.Open(artifactDigestPath(modelFile))
	if err != nil {
		return model.ArtifactDigest{}, fmt.Errorf("open artifact digest: %w", err)
	}
	defer data.Close()

	var digest model.ArtifactDigest
	var foundSHA bool
	var foundSize bool

	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "oid sha256:"):
			digest.SHA256 = strings.ToLower(strings.TrimPrefix(line, "oid sha256:"))
			foundSHA = true

		case strings.HasPrefix(line, "size "):
			sizeStr := strings.TrimPrefix(line, "size ")
			digest.Size, err = strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				return model.ArtifactDigest{}, fmt.Errorf("parse artifact size: %w", err)
			}
			foundSize = true
		}
	}

	if err := scanner.Err(); err != nil {
		return model.ArtifactDigest{}, fmt.Errorf("read artifact digest: %w", err)
	}

	decoded, err := hex.DecodeString(digest.SHA256)
	if !foundSHA || err != nil || len(decoded) != sha256.Size {
		return model.ArtifactDigest{}, fmt.Errorf("read artifact digest: invalid sha256 oid")
	}
	if !foundSize || digest.Size < 0 {
		return model.ArtifactDigest{}, fmt.Errorf("read artifact digest: invalid size")
	}

	return digest, nil
}

func readArtifactVerification(modelFile string) (model.ArtifactVerification, error) {
	raw, err := os.ReadFile(artifactVerificationPath(modelFile))
	if err != nil {
		return model.ArtifactVerification{}, fmt.Errorf("read artifact verification: %w", err)
	}

	var sentinel verifiedSentinel
	if err := json.Unmarshal(raw, &sentinel); err != nil {
		return model.ArtifactVerification{}, fmt.Errorf("decode artifact verification: %w", err)
	}
	if sentinel.VerifiedAt > 0 {
		verifiedAt := time.Unix(sentinel.VerifiedAt, 0).UTC()
		if _, err := verifiedAt.MarshalJSON(); err != nil {
			return model.ArtifactVerification{}, fmt.Errorf("decode artifact verification timestamp: %w", err)
		}
	}

	verification := model.ArtifactVerification{
		SHA256:       sentinel.SHA256,
		Size:         sentinel.Size,
		MTimeNS:      sentinel.MTimeNS,
		VerifiedAt:   sentinel.VerifiedAt,
		KronkVersion: sentinel.KronkVersion,
	}

	return verification, nil
}

func writeArtifactVerification(modelFile string, verification model.ArtifactVerification) error {
	verificationPath := artifactVerificationPath(modelFile)
	if err := os.MkdirAll(filepath.Dir(verificationPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	sentinel := verifiedSentinel{
		SHA256:       verification.SHA256,
		Size:         verification.Size,
		MTimeNS:      verification.MTimeNS,
		VerifiedAt:   verification.VerifiedAt,
		KronkVersion: verification.KronkVersion,
	}
	raw, err := json.MarshalIndent(sentinel, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(verificationPath), filepath.Base(verificationPath)+".*")
	if err != nil {
		return fmt.Errorf("tempfile: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpName, verificationPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func removeArtifactVerification(modelFile string) error {
	if err := os.Remove(artifactVerificationPath(modelFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func artifactDigestPath(modelFile string) string {
	return filepath.Join(filepath.Dir(modelFile), "sha", filepath.Base(modelFile))
}

func artifactVerificationPath(modelFile string) string {
	return artifactDigestPath(modelFile) + verifiedSuffix
}
