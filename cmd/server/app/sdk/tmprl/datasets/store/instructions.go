package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// PreparationInstructions reads the server's saved guidance. An absent file
// leaves default selection to the preparation service.
func (s *Store) PreparationInstructions(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(s.root, "preparation-instructions.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

// SavePreparationInstructions replaces guidance atomically without changing
// immutable dataset versions or instructions already captured by a run.
func (s *Store) SavePreparationInstructions(ctx context.Context, instructions string) error {
	instructions = strings.TrimSpace(instructions)
	if !utf8.ValidString(instructions) || utf8.RuneCountInString(instructions) > 2000 {
		return fmt.Errorf("%w: preparation instructions must not exceed 2000 characters", tmprl.ErrInvalidArgument)
	}
	return s.WithLock(ctx, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := os.CreateTemp(s.root, ".instructions-*.tmp")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		_, writeErr := file.WriteString(instructions)
		if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return err
		}
		if err := os.Rename(file.Name(), filepath.Join(s.root, "preparation-instructions.txt")); err != nil {
			return err
		}
		dir, err := os.Open(s.root)
		if err != nil {
			return err
		}
		return errors.Join(dir.Sync(), dir.Close())
	})
}
