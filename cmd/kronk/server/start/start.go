// Package start manages the server start sub-command.
package start

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/spf13/cobra"
)

func runLocal(cmd *cobra.Command) error {
	detach, _ := cmd.Flags().GetBool("detach")
	envVars := buildEnvVars(cmd)

	if detach {
		basePath := defaults.BaseDir(client.GetBasePath(cmd))
		liveURL, err := livenessURL(cmd)
		if err != nil {
			return err
		}

		pidFile, pidPath, err := reservePIDFile(cmd.Context(), basePath, liveURL)
		if err != nil {
			return err
		}
		keepPIDFile := false
		defer func() {
			if pidFile != nil {
				_ = pidFile.Close()
			}
			if !keepPIDFile {
				_ = os.Remove(pidPath)
			}
		}()

		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("executable: %w", err)
		}

		logFile, err := createLogFile(basePath)
		if err != nil {
			return fmt.Errorf("create log file: %w", err)
		}
		defer logFile.Close()

		proc := exec.Command(exePath, "server", "start")
		proc.Stdout = logFile
		proc.Stderr = logFile
		proc.Stdin = nil
		proc.Env = overrideEnv(os.Environ(), envVars)
		setDetachAttrs(proc)

		if err := proc.Start(); err != nil {
			return fmt.Errorf("start: %w", err)
		}

		if _, err := fmt.Fprint(pidFile, proc.Process.Pid); err != nil {
			killErr := proc.Process.Kill()
			return errors.Join(fmt.Errorf("write pid file: %w", err), killErr)
		}
		if err := pidFile.Sync(); err != nil {
			killErr := proc.Process.Kill()
			return errors.Join(fmt.Errorf("sync pid file: %w", err), killErr)
		}
		if err := pidFile.Close(); err != nil {
			killErr := proc.Process.Kill()
			return errors.Join(fmt.Errorf("close pid file: %w", err), killErr)
		}
		pidFile = nil
		keepPIDFile = true

		if waitForLiveness(cmd.Context(), liveURL, 3*time.Second) {
			fmt.Printf("Kronk server started in background (PID: %d)\n", proc.Process.Pid)
			return nil
		}
		if !processAlive(proc.Process.Pid) {
			keepPIDFile = false
			return fmt.Errorf("kronk server exited during startup; inspect %s", logFilePath(basePath))
		}

		fmt.Printf("Kronk server launched in background (PID: %d); startup is still in progress\n", proc.Process.Pid)
		fmt.Printf("Follow startup with: kronk --base-path %q server logs\n", basePath)
		return nil
	}

	for _, env := range envVars {
		parts := splitEnvVar(env)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}

	if err := kronk.Run(false); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

func buildEnvVars(cmd *cobra.Command) []string {
	var envVars []string

	addString := func(flag string, env string) {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			envVars = append(envVars, env+"="+value)
		}
	}
	addBool := func(flag string, env string) {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetBool(flag)
			envVars = append(envVars, env+"="+strconv.FormatBool(value))
		}
	}
	addInt := func(flag string, env string) {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetInt(flag)
			envVars = append(envVars, env+"="+strconv.Itoa(value))
		}
	}
	addFloat := func(flag string, env string) {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetFloat64(flag)
			envVars = append(envVars, env+"="+strconv.FormatFloat(value, 'f', -1, 64))
		}
	}
	addStringSlice := func(flag string, env string) {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetStringSlice(flag)
			envVars = append(envVars, env+"="+strings.Join(value, ","))
		}
	}

	// Web settings
	addString("api-host", "KRONK_WEB_API_HOST")
	addString("debug-host", "KRONK_WEB_DEBUG_HOST")
	addString("read-timeout", "KRONK_WEB_READ_TIMEOUT")
	addString("inference-timeout", "KRONK_WEB_INFERENCE_TIMEOUT")
	addString("write-timeout", "KRONK_WEB_WRITE_TIMEOUT")
	addString("idle-timeout", "KRONK_WEB_IDLE_TIMEOUT")
	addString("shutdown-timeout", "KRONK_WEB_SHUTDOWN_TIMEOUT")
	addStringSlice("cors-allowed-origins", "KRONK_WEB_CORS_ALLOWED_ORIGINS")

	// Auth settings
	addBool("web-admin-enabled", "KRONK_WEB_ADMIN_ENABLED")
	addString("auth-host", "KRONK_AUTH_HOST")
	addBool("auth-tls-enabled", "KRONK_AUTH_TLS_ENABLED")
	addString("auth-tls-ca-file", "KRONK_AUTH_TLS_CA_FILE")
	addString("auth-tls-server-name", "KRONK_AUTH_TLS_SERVER_NAME")
	addString("auth-issuer", "KRONK_AUTH_LOCAL_ISSUER")
	addString("web-admin-password-sha-256", "KRONK_WEB_ADMIN_PASSWORD_SHA_256")
	addString("authorization-mode", "KRONK_AUTHORIZATION_MODE")

	// MCP settings
	addBool("mcp-enabled", "KRONK_MCP_ENABLED")
	addString("mcp-host", "KRONK_MCP_HOST")
	addBool("mcp-auth-enabled", "KRONK_MCP_AUTH_ENABLED")
	addString("mcp-brave-api-key", "KRONK_MCP_BRAVE_API_KEY")
	addBool("download-enabled", "KRONK_DOWNLOAD_ENABLED")

	// Tempo/tracing settings
	addString("tempo-host", "KRONK_TEMPO_HOST")
	addString("tempo-service-name", "KRONK_TEMPO_SERVICE_NAME")
	addFloat("tempo-probability", "KRONK_TEMPO_PROBABILITY")

	// Pool settings
	addString("model-config-file", "KRONK_POOL_MODEL_CONFIG_FILE")
	addInt("budget-percent", "KRONK_POOL_BUDGET_PERCENT")
	addInt("models-in-pool", "KRONK_POOL_MODELS_IN_POOL")
	addString("pool-ttl", "KRONK_POOL_TTL")

	// Runtime settings
	addString("base-path", "KRONK_BASE_PATH")
	addString("lib-path", "KRONK_LIB_PATH")
	addString("bucky-lib-path", "KRONK_BUCKY_LIB_PATH")
	addString("malina-lib-path", "KRONK_MALINA_LIB_PATH")
	addString("lib-version", "KRONK_LIB_VERSION")
	addBool("lib-download-enabled", "KRONK_LIB_DOWNLOAD_ENABLED")
	addBool("media-backends-enabled", "KRONK_MEDIA_BACKENDS_ENABLED")
	addBool("lib-verify-enabled", "KRONK_LIB_VERIFY_ENABLED")
	addString("arch", "KRONK_ARCH")
	addString("os", "KRONK_OS")
	addString("processor", "KRONK_PROCESSOR")
	addString("hf-token", "KRONK_HF_TOKEN")
	addBool("allow-upgrade", "KRONK_ALLOW_UPGRADE")
	addInt("llama-log", "KRONK_LLAMA_LOG")
	addBool("insecure-logging", "KRONK_INSECURE_LOGGING")

	return envVars
}

func overrideEnv(base []string, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))

	for _, env := range append(append([]string{}, base...), overrides...) {
		parts := splitEnvVar(env)
		if len(parts) != 2 {
			continue
		}
		if _, exists := values[parts[0]]; !exists {
			order = append(order, parts[0])
		}
		values[parts[0]] = parts[1]
	}

	env := make([]string, 0, len(order))
	for _, key := range order {
		env = append(env, key+"="+values[key])
	}

	return env
}

func splitEnvVar(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}

func livenessURL(cmd *cobra.Command) (string, error) {
	host, _ := cmd.Flags().GetString("api-host")
	if host == "" {
		host = os.Getenv("KRONK_WEB_API_HOST")
	}
	if host == "" {
		host = "localhost:11435"
	}
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}

	liveURL, err := url.JoinPath(host, "/v1/liveness")
	if err != nil {
		return "", fmt.Errorf("liveness url: %w", err)
	}
	return liveURL, nil
}

func serverLive(ctx context.Context, liveURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, liveURL, nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func waitForLiveness(ctx context.Context, liveURL string, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if serverLive(ctx, liveURL) {
			return true
		}

		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func reservePIDFile(ctx context.Context, basePath string, liveURL string) (*os.File, string, error) {
	path := pidFilePath(basePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", fmt.Errorf("create pid directory: %w", err)
	}

	if serverLive(ctx, liveURL) {
		return nil, "", fmt.Errorf("kronk server is already running at %s", liveURL)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		return file, path, nil
	}
	if !os.IsExist(err) {
		return nil, "", fmt.Errorf("reserve pid file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read existing pid file: %w", err)
	}
	pidText := strings.TrimSpace(string(data))
	if pidText == "" {
		return nil, "", fmt.Errorf("another Kronk server start is already in progress")
	}
	pid, err := strconv.Atoi(pidText)
	if err == nil && pid > 0 && processAlive(pid) {
		return nil, "", fmt.Errorf("kronk server process %d is already running or starting; inspect %s", pid, logFilePath(basePath))
	}

	if err := os.Remove(path); err != nil {
		return nil, "", fmt.Errorf("remove stale pid file: %w", err)
	}
	return nil, "", fmt.Errorf("removed stale pid file; run the start command again")
}

func logFilePath(basePath string) string {
	return filepath.Join(basePath, "kronk.log")
}

func createLogFile(basePath string) (*os.File, error) {
	path := logFilePath(basePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	return os.Create(path)
}

func pidFilePath(basePath string) string {
	return filepath.Join(basePath, "kronk.pid")
}
