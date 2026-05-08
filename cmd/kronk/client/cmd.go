package client

import (
	"fmt"
	"net/url"
	"os"
	"strings"

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
//
// KRONK_WEB_API_HOST is shared with the server, which expects a bare
// host:port (e.g. "127.0.0.1:11435"). Accept that form here as well by
// defaulting the scheme to http:// when one isn't provided, so the same
// env var works for both the server and the CLI.
func DefaultURL(path string) (string, error) {
	host := "http://localhost:11435"
	if v := os.Getenv("KRONK_WEB_API_HOST"); v != "" {
		host = v
		if !strings.Contains(host, "://") {
			host = "http://" + host
		}
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
