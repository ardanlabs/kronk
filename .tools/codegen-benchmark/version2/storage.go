package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
)

func latestRun(runsDir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(runsDir, "*", "run-*"))
	if err != nil {
		return "", err
	}
	paths = slices.DeleteFunc(paths, func(path string) bool {
		info, err := os.Stat(path)
		return err != nil || !info.IsDir()
	})
	if len(paths) == 0 {
		return "", errors.New("codegen benchmark: no saved runs; specify -run for a legacy output directory")
	}
	slices.Sort(paths)
	return paths[len(paths)-1], nil
}
