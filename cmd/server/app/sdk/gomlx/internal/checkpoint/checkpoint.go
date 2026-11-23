// Package checkpoint saves and validates GoMLX adapter checkpoints.
package checkpoint

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gomlx/gomlx/core/tensors"
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

// TrainingVariable is a serialized runtime variable, not an inference export.
type TrainingVariable struct {
	Path      string
	Trainable bool
	Tensor    []byte
}

type trainingDocument struct {
	Version   int
	State     []byte
	Variables []TrainingVariable
}

// ErrNoTrainingCheckpoint means no manifest has ever been committed.
var ErrNoTrainingCheckpoint = errors.New("no committed training checkpoint")

// SaveTraining commits an immutable, checksummed generation before atomically
// replacing the latest pointer. The returned file path is relative to directory.
// excluded must contain every frozen base variable; they are never serialized.
// A directory belongs to exactly one writer. Old generations remain immutable.
func SaveTraining(directory string, store *model.Store, excluded map[string]bool, state []byte) (string, error) {
	document := trainingDocument{Version: 1, State: state}
	for variable := range store.IterVariables() {
		// GoMLX display metrics use per-trainer random UUID scopes. They are
		// not optimizer inputs; epoch accumulators belong to the caller state.
		if excluded[variable.Path()] || strings.HasPrefix(variable.Path(), "/metrics/") {
			continue
		}
		value, err := variable.Value()
		if err != nil {
			return "", err
		}
		var buffer bytes.Buffer
		if err := value.GobSerialize(gob.NewEncoder(&buffer)); err != nil {
			return "", err
		}
		document.Variables = append(document.Variables, TrainingVariable{Path: variable.Path(), Trainable: variable.Trainable, Tensor: buffer.Bytes()})
	}
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(document); err != nil {
		return "", err
	}
	payload := buffer.Bytes()
	digest := sha256.Sum256(payload)
	payload = append(payload, digest[:]...)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := SyncDirectory(filepath.Dir(filepath.Clean(directory))); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, "training-*.chk")
	if err != nil {
		return "", err
	}
	name := filepath.Base(file.Name())
	if err := writeSynced(file, payload); err != nil {
		return "", err
	}
	// A generation is not visible to readers until its complete contents and
	// directory entry are durable and the latest pointer has been committed.
	if err := SyncDirectory(directory); err != nil {
		return "", err
	}
	pointer, err := json.Marshal(name)
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".latest-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary.Name())
	if err := writeSynced(temporary, pointer); err != nil {
		return "", err
	}
	if err := os.Rename(temporary.Name(), filepath.Join(directory, "training-latest.json")); err != nil {
		return "", err
	}
	if err := SyncDirectory(directory); err != nil {
		return "", err
	}
	return name, nil
}

func writeSynced(file *os.File, data []byte) error {
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	return errors.Join(writeErr, syncErr, file.Close())
}

// SyncDirectory makes prior directory-entry changes durable.
func SyncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

// SyncTree flushes an inference checkpoint before a training generation refers
// to it. SaveAdapter's inference-only format and overwrite rules are unchanged.
func SyncTree(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular checkpoint entry %q", entry.Name())
		}
		file, err := os.Open(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		if err := errors.Join(file.Sync(), file.Close()); err != nil {
			return err
		}
	}
	return errors.Join(SyncDirectory(directory), SyncDirectory(filepath.Dir(directory)))
}

// ReadTraining reads only the committed generation. A missing latest pointer
// with existing generations fails closed, never resets potentially saved work.
// Corrupt committed progress is an error, never a silent restart from zero.
func ReadTraining(directory string) ([]byte, []TrainingVariable, error) {
	pointer, err := os.ReadFile(filepath.Join(directory, "training-latest.json"))
	if os.IsNotExist(err) {
		entries, readErr := os.ReadDir(directory)
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, nil, readErr
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "training-") && strings.HasSuffix(entry.Name(), ".chk") {
				return nil, nil, fmt.Errorf("training manifest is missing while saved generations exist")
			}
		}
		return nil, nil, ErrNoTrainingCheckpoint
	}
	if err != nil {
		return nil, nil, err
	}
	var name string
	if err := json.Unmarshal(pointer, &name); err != nil {
		return nil, nil, err
	}
	if filepath.Base(name) != name || !strings.HasPrefix(name, "training-") || !strings.HasSuffix(name, ".chk") {
		return nil, nil, fmt.Errorf("invalid training generation %q", name)
	}
	payload, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		return nil, nil, fmt.Errorf("read committed training generation: %w", err)
	}
	if len(payload) < sha256.Size {
		return nil, nil, fmt.Errorf("truncated training generation")
	}
	data := payload[:len(payload)-sha256.Size]
	digest := sha256.Sum256(data)
	if !bytes.Equal(digest[:], payload[len(data):]) {
		return nil, nil, fmt.Errorf("training generation checksum mismatch")
	}
	var document trainingDocument
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&document); err != nil {
		return nil, nil, err
	}
	if document.Version != 1 {
		return nil, nil, fmt.Errorf("unsupported training checkpoint version %d", document.Version)
	}
	return document.State, document.Variables, nil
}

// RestoreTraining installs previously validated state in a fresh runtime. It
// rejects base weights and duplicate names before modifying the model store.
func RestoreTraining(store *model.Store, excluded map[string]bool, variables []TrainingVariable) error {
	values := make([]*tensors.Tensor, len(variables))
	defer func() {
		for _, value := range values {
			if value != nil {
				value.FinalizeAll()
			}
		}
	}()
	seen := make(map[string]bool)
	for index, variable := range variables {
		if excluded[variable.Path] || variable.Path == "" || seen[variable.Path] {
			return fmt.Errorf("invalid training variable %q", variable.Path)
		}
		seen[variable.Path] = true
		value, err := tensors.GobDeserialize(gob.NewDecoder(bytes.NewReader(variable.Tensor)))
		if err != nil {
			return fmt.Errorf("decode training variable %q: %w", variable.Path, err)
		}
		values[index] = value
		if existing := store.GetVariable(variable.Path); existing != nil && !existing.Shape().Equal(value.Shape()) {
			return fmt.Errorf("incompatible training variable %q", variable.Path)
		}
	}
	for index, variable := range variables {
		existing := store.GetVariable(variable.Path)
		if existing == nil {
			var err error
			existing, err = store.CreateVariable(variable.Path, values[index])
			if err != nil {
				return err
			}
		} else if err := existing.SetValue(values[index]); err != nil {
			return err
		}
		existing.SetTrainable(variable.Trainable)
		values[index] = nil // Ownership passed to the store.
	}
	return nil
}
