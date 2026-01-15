package model

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CheckModel is check if the downloaded model is valid based on it's sha
// file. If no sha file exists, this check will return with no error.
func CheckModel(modelFile string, checkSHA bool) error {
	dir := filepath.Dir(modelFile)
	base := filepath.Base(modelFile)
	shaFile := filepath.Join(dir, "sha", base)

	data, err := os.Open(shaFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("check-model: opening sha file: %w", err)
	}
	defer data.Close()

	var expectedSHA string
	var expectedSize int64

	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "oid sha256:"):
			expectedSHA = strings.TrimPrefix(line, "oid sha256:")

		case strings.HasPrefix(line, "size "):
			sizeStr := strings.TrimPrefix(line, "size ")
			expectedSize, err = strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				return fmt.Errorf("check-model: parsing size from sha file: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("check-model: reading sha file: %w", err)
	}

	info, err := os.Stat(modelFile)
	if err != nil {
		return fmt.Errorf("check-model: stat model file: %w", err)
	}

	if info.Size() != expectedSize {
		return fmt.Errorf("check-model: size mismatch: expected %d, got %d", expectedSize, info.Size())
	}

	if checkSHA {
		f, err := os.Open(modelFile)
		if err != nil {
			return fmt.Errorf("check-model: opening model file for sha check: %w", err)
		}
		defer f.Close()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("check-model: computing sha256: %w", err)
		}

		actualSHA := fmt.Sprintf("%x", h.Sum(nil))
		if actualSHA != expectedSHA {
			return fmt.Errorf("check-model: sha256 mismatch: expected %s, got %s", expectedSHA, actualSHA)
		}
	}

	return nil
}
