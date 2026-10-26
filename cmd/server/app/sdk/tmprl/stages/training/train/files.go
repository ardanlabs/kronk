package train

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/dataset"
)

// CopyArtifact atomically publishes a nonempty artifact of at most 1 GiB.
func CopyArtifact(ctx context.Context, root *os.Root, path string, reader io.Reader) error {
	temporary := ".artifact-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	size, copyErr := io.Copy(file, io.LimitReader(dataset.ContextReader{Context: ctx, Reader: reader}, (1<<30)+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	if size == 0 || size > 1<<30 {
		return fmt.Errorf("invalid training artifact size")
	}
	return root.Rename(temporary, path)
}
