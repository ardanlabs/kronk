// Package checkpoint saves and validates GoMLX adapter checkpoints.
package checkpoint

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gomlx/gomlx/ml/model"
	gomlxcheckpoint "github.com/gomlx/gomlx/ml/model/checkpoint"
	"github.com/gomlx/gomlx/ml/train/optimizer"
)

type metadata struct {
	Variables []variable
	BinFormat string
}

type variable struct {
	ParameterName string
	Dimensions    []int
	DType         string
	Pos           int64
	Length        int64
}

// SaveAdapter writes a new inference-only checkpoint containing LoRA A/B
// tensors. It refuses to replace any existing path and validates its output.
func SaveAdapter(directory string, store *model.Store) error {
	if directory == "" {
		return errors.New("adapter checkpoint directory is required")
	}
	if store == nil {
		return errors.New("model store is required")
	}
	if _, err := os.Lstat(directory); err == nil {
		return fmt.Errorf("adapter checkpoint path %q already exists", directory)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect adapter checkpoint path %q: %w", directory, err)
	}

	expected := make(map[string][]int)
	var excluded []*model.Variable
	for value := range store.IterVariables() {
		if strings.HasSuffix(value.Path(), "/lora/a") || strings.HasSuffix(value.Path(), "/lora/b") {
			expected[value.Path()] = slices.Clone(value.Shape().Dimensions)
			continue
		}
		excluded = append(excluded, value)
	}
	if len(expected) == 0 {
		return errors.New("model contains no LoRA A/B variables")
	}

	handler, err := gomlxcheckpoint.Build(store).
		Dir(directory).
		Keep(1).
		ExcludeAllParams().
		ExcludeVars(excluded...).
		Done()
	if err != nil {
		return fmt.Errorf("create adapter checkpoint %q: %w", directory, err)
	}
	handler.ExcludeVarsFromSaving(optimizer.GetGlobalStepVar(store))
	if err := handler.Save(); err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("save adapter checkpoint %q: %w", directory, err)
	}
	if err := Validate(directory, expected); err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("validate saved adapter checkpoint %q: %w", directory, err)
	}
	return nil
}

// Validate requires the latest checkpoint to contain exactly the expected F32
// variables and a binary payload consistent with its metadata.
func Validate(directory string, expected map[string][]int) error {
	if directory == "" {
		return errors.New("checkpoint directory is required")
	}
	if len(expected) == 0 {
		return errors.New("expected checkpoint variables are required")
	}
	base, err := latest(directory)
	if err != nil {
		return err
	}
	metadataPath := filepath.Join(directory, base+".json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("read checkpoint metadata %q: %w", metadataPath, err)
	}
	var document metadata
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode checkpoint metadata %q: %w", metadataPath, err)
	}
	if document.BinFormat != "" && document.BinFormat != "uncompressed" && document.BinFormat != "gzip" {
		return fmt.Errorf("checkpoint uses unsupported binary format %q", document.BinFormat)
	}

	remaining := make(map[string][]int, len(expected))
	for name, shape := range expected {
		remaining[name] = slices.Clone(shape)
	}
	var end int64
	for _, value := range document.Variables {
		name, ok := strings.CutPrefix(value.ParameterName, "var:")
		if !ok || name == "" {
			return fmt.Errorf("checkpoint variable has invalid parameter name %q", value.ParameterName)
		}
		shape, ok := remaining[name]
		if !ok {
			return fmt.Errorf("checkpoint contains unexpected variable %q", name)
		}
		if value.DType != "Float32" || !slices.Equal(value.Dimensions, shape) {
			return fmt.Errorf("checkpoint variable %q is %s %v, want Float32 %v", name, value.DType, value.Dimensions, shape)
		}
		length, err := float32Bytes(shape)
		if err != nil {
			return fmt.Errorf("checkpoint variable %q: %w", name, err)
		}
		if value.Pos != end || value.Length != length {
			return fmt.Errorf("checkpoint variable %q has payload range [%d,%d), want [%d,%d)", name, value.Pos, value.Pos+value.Length, end, end+length)
		}
		end += length
		delete(remaining, name)
	}
	if len(remaining) != 0 {
		names := make([]string, 0, len(remaining))
		for name := range remaining {
			names = append(names, name)
		}
		slices.Sort(names)
		return fmt.Errorf("checkpoint is missing variable %q", names[0])
	}

	binaryPath := filepath.Join(directory, base+".bin")
	info, err := os.Lstat(binaryPath)
	if err != nil {
		return fmt.Errorf("inspect checkpoint payload %q: %w", binaryPath, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("checkpoint payload %q is not a regular file", binaryPath)
	}
	if document.BinFormat == "gzip" {
		return validateGzipPayload(binaryPath)
	}
	if info.Size() != end {
		return fmt.Errorf("checkpoint payload has %d bytes, metadata requires %d", info.Size(), end)
	}
	return nil
}

func validateGzipPayload(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open checkpoint payload %q: %w", path, err)
	}
	const containerHeader = "gomlx_checkpointss"
	header := make([]byte, len(containerHeader)+1+len("gzip"))
	_, gzipErr := io.ReadFull(file, header)
	if gzipErr == nil && (string(header[:len(containerHeader)]) != containerHeader ||
		int(header[len(containerHeader)]) != len("gzip") || string(header[len(containerHeader)+1:]) != "gzip") {
		gzipErr = errors.New("invalid GoMLX checkpoint header")
	}
	var compressed *gzip.Reader
	if gzipErr == nil {
		compressed, gzipErr = gzip.NewReader(file)
	}
	if gzipErr == nil {
		gzipErr = compressed.Close()
	}
	closeErr := file.Close()
	if gzipErr != nil {
		return fmt.Errorf("checkpoint payload is not a valid gzip stream: %w", gzipErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close checkpoint payload: %w", closeErr)
	}
	return nil
}

func latest(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("read checkpoint directory %q: %w", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "checkpoint-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".json"))
	}
	if len(names) == 0 {
		return "", fmt.Errorf("checkpoint directory %q contains no checkpoint metadata", directory)
	}
	slices.Sort(names)
	return names[len(names)-1], nil
}

func float32Bytes(shape []int) (int64, error) {
	count := int64(1)
	for _, dimension := range shape {
		if dimension <= 0 || count > math.MaxInt64/int64(dimension) {
			return 0, fmt.Errorf("invalid shape %v", shape)
		}
		count *= int64(dimension)
	}
	if count > math.MaxInt64/4 {
		return 0, fmt.Errorf("shape %v is too large", shape)
	}
	return count * 4, nil
}
