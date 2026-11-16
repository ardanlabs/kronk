package disk

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func lockDirectory(directory string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(directory, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped)); err != nil {
		return nil, errors.Join(err, f.Close())
	}

	return f, nil
}

// Windows does not support fsync on directory handles through os.File.Sync.
func syncDirectory(string) error {
	return nil
}
