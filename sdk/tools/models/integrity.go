package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// IntegrityStatus describes the available verification evidence for a model
// or artifact.
type IntegrityStatus string

const (
	// IntegrityVerified means the artifact's published digest and persisted
	// verification match its current filesystem metadata.
	IntegrityVerified IntegrityStatus = "verified"

	// IntegrityUnverified means a published digest exists but no persisted
	// verification exists.
	IntegrityUnverified IntegrityStatus = "unverified"

	// IntegrityStale means persisted verification evidence or current file
	// metadata no longer matches the published artifact metadata.
	IntegrityStale IntegrityStatus = "stale"

	// IntegrityUnavailable means no trustworthy published digest is available.
	IntegrityUnavailable IntegrityStatus = "unavailable"
)

const (
	// IntegrityRoleWeights identifies a model weight file or shard.
	IntegrityRoleWeights = "weights"

	// IntegrityRoleProjection identifies a multimodal projection artifact.
	IntegrityRoleProjection = "projection"

	// IntegrityRoleMTP identifies an MTP drafter artifact.
	IntegrityRoleMTP = "mtp"
)

const (
	// IntegrityReasonDigestUnavailable means the artifact's Git LFS pointer is
	// absent or malformed.
	IntegrityReasonDigestUnavailable = "digest_unavailable"

	// IntegrityReasonFileMetadataChanged means verification evidence does not
	// match the published digest or current filesystem metadata.
	IntegrityReasonFileMetadataChanged = "file_metadata_changed"
)

// IntegrityArtifact provides the identity and persisted verification evidence
// for one physical model artifact.
type IntegrityArtifact struct {
	Role       string
	Filename   string
	Digest     string
	Size       int64
	Status     IntegrityStatus
	Verified   bool
	VerifiedAt time.Time
	Reason     string
}

// IntegrityModel provides integrity information for one indexed model and all
// of its required artifacts.
type IntegrityModel struct {
	ID          string
	OwnedBy     string
	ModelFamily string
	Status      IntegrityStatus
	Verified    bool
	Artifacts   []IntegrityArtifact
}

// Integrity returns the local integrity inventory without hashing artifacts,
// rebuilding the model index, parsing GGUF data, or accessing the network.
func (m *Models) Integrity() ([]IntegrityModel, error) {
	index, err := m.readIndex()
	if err != nil {
		return nil, fmt.Errorf("integrity: %w", err)
	}

	list := make([]IntegrityModel, 0, len(index))
	for modelID, mp := range index {
		if len(mp.ModelFiles) == 0 {
			continue
		}

		modelInfo, err := m.integrityModel(modelID, mp)
		if err != nil {
			return nil, err
		}
		list = append(list, modelInfo)
	}

	slices.SortFunc(list, func(a, b IntegrityModel) int {
		if order := strings.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID)); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})

	return list, nil
}

// IntegrityFor returns the local integrity information for one indexed model
// without inspecting artifacts belonging to other models.
func (m *Models) IntegrityFor(modelID string) (IntegrityModel, error) {
	index, err := m.readIndex()
	if err != nil {
		return IntegrityModel{}, fmt.Errorf("integrity-for: %w", err)
	}

	for _, key := range fullPathLookupKeys(modelID) {
		mp, exists := index[key]
		if !exists || len(mp.ModelFiles) == 0 {
			continue
		}

		return m.integrityModel(key, mp)
	}

	return IntegrityModel{}, fmt.Errorf("integrity-for: %w: %q", ErrModelNotFound, modelID)
}

func (m *Models) integrityModel(modelID string, mp Path) (IntegrityModel, error) {
	modelInfo := IntegrityModel{
		ID:        modelID,
		Status:    IntegrityVerified,
		Verified:  true,
		Artifacts: make([]IntegrityArtifact, 0, len(mp.ModelFiles)+2),
	}
	modelInfo.OwnedBy, modelInfo.ModelFamily = m.artifactOwnership(mp.ModelFiles[0])

	modelFiles := slices.Clone(mp.ModelFiles)
	slices.Sort(modelFiles)
	for _, modelFile := range modelFiles {
		artifact, err := integrityArtifact(modelFile, IntegrityRoleWeights)
		if err != nil {
			return IntegrityModel{}, fmt.Errorf("integrity: model %q: %w", modelID, err)
		}
		modelInfo.Artifacts = append(modelInfo.Artifacts, artifact)
	}

	if mp.ProjFile != "" {
		artifact, err := integrityArtifact(mp.ProjFile, IntegrityRoleProjection)
		if err != nil {
			return IntegrityModel{}, fmt.Errorf("integrity: model %q: %w", modelID, err)
		}
		modelInfo.Artifacts = append(modelInfo.Artifacts, artifact)
	}

	if mp.MTPFile != "" {
		artifact, err := integrityArtifact(mp.MTPFile, IntegrityRoleMTP)
		if err != nil {
			return IntegrityModel{}, fmt.Errorf("integrity: model %q: %w", modelID, err)
		}
		modelInfo.Artifacts = append(modelInfo.Artifacts, artifact)
	}

	for _, artifact := range modelInfo.Artifacts {
		modelInfo.Status = aggregateIntegrityStatus(modelInfo.Status, artifact.Status)
		modelInfo.Verified = modelInfo.Verified && artifact.Verified
	}

	return modelInfo, nil
}

func (m *Models) artifactOwnership(artifactPath string) (string, string) {
	relativePath, err := filepath.Rel(m.modelsPath, artifactPath)
	if err != nil || relativePath == "." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", ""
	}

	parts := strings.Split(relativePath, string(filepath.Separator))
	if len(parts) < 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

func integrityArtifact(artifactPath string, role string) (IntegrityArtifact, error) {
	artifact := IntegrityArtifact{
		Role:     role,
		Filename: filepath.Base(artifactPath),
	}

	info, statErr := os.Stat(artifactPath)
	if statErr == nil {
		artifact.Size = info.Size()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return IntegrityArtifact{}, fmt.Errorf("stat artifact %q: %w", artifact.Filename, statErr)
	}

	digest, err := readArtifactDigest(artifactPath)
	if err != nil {
		artifact.Status = IntegrityUnavailable
		artifact.Reason = IntegrityReasonDigestUnavailable
		return artifact, nil
	}
	artifact.Digest = "sha256:" + digest.SHA256

	verification, err := readArtifactVerification(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && statErr == nil && info.Size() == digest.Size {
			artifact.Status = IntegrityUnverified
			return artifact, nil
		}

		artifact.Status = IntegrityStale
		artifact.Reason = IntegrityReasonFileMetadataChanged
		return artifact, nil
	}

	if verification.VerifiedAt > 0 {
		artifact.VerifiedAt = time.Unix(verification.VerifiedAt, 0).UTC()
	}

	if statErr != nil ||
		!strings.EqualFold(verification.SHA256, digest.SHA256) ||
		verification.Size != digest.Size ||
		info.Size() != verification.Size ||
		info.ModTime().UnixNano() != verification.MTimeNS {
		artifact.Status = IntegrityStale
		artifact.Reason = IntegrityReasonFileMetadataChanged
		return artifact, nil
	}

	artifact.Status = IntegrityVerified
	artifact.Verified = true
	return artifact, nil
}

// aggregateIntegrityStatus chooses the least trustworthy artifact status for
// a model: unavailable, then stale, then unverified, then verified.
func aggregateIntegrityStatus(current IntegrityStatus, next IntegrityStatus) IntegrityStatus {
	severity := map[IntegrityStatus]int{
		IntegrityVerified:    0,
		IntegrityUnverified:  1,
		IntegrityStale:       2,
		IntegrityUnavailable: 3,
	}

	if severity[next] > severity[current] {
		return next
	}

	return current
}
