//go:build unix

package disk

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockDirectory(directory string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(directory, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Join(err, f.Close())
	}

	return f, nil
}

func syncDirectory(directory string) error {
	f, err := os.Open(directory)
	if err != nil {
		return err
	}

	return errors.Join(f.Sync(), f.Close())
}
