package client

import (
	"fmt"
	"net/url"
	"os"
)

// DefaultURL is a convience function for getting a url using the
// default local model server host:port or pulling from KRONK_HOST.
func DefaultURL(path string) (string, error) {
	host := "http://127.0.0.1:3000"
	if v := os.Getenv("KRONK_HOST"); v != "" {
		host = v
	}

	path, err := url.JoinPath(host, path)
	if err != nil {
		return "", fmt.Errorf("run-web: join-path: %w", err)
	}

	if _, err := url.Parse(path); err != nil {
		return "", fmt.Errorf("run-web: url path not valid: %w", err)
	}

	return path, nil
}
