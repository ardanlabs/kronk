package client

import (
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// GetBasePath returns the base path for kronk data, checking the
// --base-path flag first, then falling back to KRONK_BASE_PATH env var.
func GetBasePath(cmd *cobra.Command) string {
	if cmd != nil {
		if basePath, _ := cmd.Flags().GetString("base-path"); basePath != "" {
			return basePath
		}
	}
	return os.Getenv("KRONK_BASE_PATH")
}

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
