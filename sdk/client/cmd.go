package client

import (
	"fmt"
	"net/url"
	"os"
)

// DefaultURL is a convience function for getting a url using the
// default local model server host:port or pulling from KRONK_WEB_API_HOST.
func DefaultURL(path string) (string, error) {
	host := "http://localhost:8080"
	if v := os.Getenv("KRONK_WEB_API_HOST"); v != "" {
		host = v
	}

	path, err := url.JoinPath(host, path)
	if err != nil {
		return "", fmt.Errorf("default-url: join path, host[%s] path[%s]: %w", host, path, err)
	}

	if _, err := url.Parse(path); err != nil {
		return "", fmt.Errorf("default-url: parse, path[%s]: %w", path, err)
	}

	return path, nil
}
