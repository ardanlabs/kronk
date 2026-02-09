// Package start manages the server start sub-command.
package start

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/spf13/cobra"
)

func runLocal(cmd *cobra.Command) error {
	detach, _ := cmd.Flags().GetBool("detach")

	envVars := buildEnvVars(cmd)

	if detach {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("executable: %w", err)
		}

		logFile, _ := os.Create(logFilePath())

		proc := exec.Command(exePath, "server", "start")
		proc.Stdout = logFile
		proc.Stderr = logFile
		proc.Stdin = nil
		proc.Env = append(os.Environ(), envVars...)
		setDetachAttrs(proc)

		if err := proc.Start(); err != nil {
			return fmt.Errorf("start: %w", err)
		}

		pidFile := pidFilePath()
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(proc.Process.Pid)), 0644); err != nil {
			return fmt.Errorf("failed to write pid file: %w", err)
		}

		fmt.Printf("Kronk server started in background (PID: %d)\n", proc.Process.Pid)

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

	// Web settings
	if v, _ := cmd.Flags().GetString("api-host"); v != "" {
		envVars = append(envVars, "KRONK_WEB_API_HOST="+v)
	}
	if v, _ := cmd.Flags().GetString("debug-host"); v != "" {
		envVars = append(envVars, "KRONK_WEB_DEBUG_HOST="+v)
	}
	if v, _ := cmd.Flags().GetString("read-timeout"); v != "" {
		envVars = append(envVars, "KRONK_WEB_READ_TIMEOUT="+v)
	}
	if v, _ := cmd.Flags().GetString("write-timeout"); v != "" {
		envVars = append(envVars, "KRONK_WEB_WRITE_TIMEOUT="+v)
	}
	if v, _ := cmd.Flags().GetString("idle-timeout"); v != "" {
		envVars = append(envVars, "KRONK_WEB_IDLE_TIMEOUT="+v)
	}
	if v, _ := cmd.Flags().GetString("shutdown-timeout"); v != "" {
		envVars = append(envVars, "KRONK_WEB_SHUTDOWN_TIMEOUT="+v)
	}
	if v, _ := cmd.Flags().GetStringSlice("cors-allowed-origins"); len(v) > 0 {
		envVars = append(envVars, "KRONK_WEB_CORS_ALLOWED_ORIGINS="+strings.Join(v, ","))
	}

	// Auth settings
	if cmd.Flags().Changed("auth-enabled") {
		v, _ := cmd.Flags().GetBool("auth-enabled")
		envVars = append(envVars, "KRONK_AUTH_LOCAL_ENABLED="+strconv.FormatBool(v))
	}
	if v, _ := cmd.Flags().GetString("auth-host"); v != "" {
		envVars = append(envVars, "KRONK_AUTH_HOST="+v)
	}
	if v, _ := cmd.Flags().GetString("auth-issuer"); v != "" {
		envVars = append(envVars, "KRONK_AUTH_LOCAL_ISSUER="+v)
	}

	// Tempo/tracing settings
	if v, _ := cmd.Flags().GetString("tempo-host"); v != "" {
		envVars = append(envVars, "KRONK_TEMPO_HOST="+v)
	}
	if v, _ := cmd.Flags().GetString("tempo-service-name"); v != "" {
		envVars = append(envVars, "KRONK_TEMPO_SERVICE_NAME="+v)
	}
	if v, _ := cmd.Flags().GetFloat64("tempo-probability"); v >= 0 {
		envVars = append(envVars, "KRONK_TEMPO_PROBABILITY="+strconv.FormatFloat(v, 'f', -1, 64))
	}

	// Catalog settings
	if v, _ := cmd.Flags().GetString("catalog-github-repo"); v != "" {
		envVars = append(envVars, "KRONK_CATALOG_GITHUB_REPO="+v)
	}
	if v, _ := cmd.Flags().GetString("model-config-file"); v != "" {
		envVars = append(envVars, "KRONK_CATALOG_MODEL_CONFIG_FILE="+v)
	}
	if v, _ := cmd.Flags().GetString("catalog-repo-path"); v != "" {
		envVars = append(envVars, "KRONK_CATALOG_REPO_PATH="+v)
	}

	// Templates settings
	if v, _ := cmd.Flags().GetString("templates-github-repo"); v != "" {
		envVars = append(envVars, "KRONK_TEMPLATES_GITHUB_REPO="+v)
	}

	// Cache settings
	if v, _ := cmd.Flags().GetInt("model-instances"); v != 0 {
		envVars = append(envVars, "KRONK_CACHE_MODEL_INSTANCES="+strconv.Itoa(v))
	}
	if v, _ := cmd.Flags().GetInt("models-in-cache"); v != 0 {
		envVars = append(envVars, "KRONK_CACHE_MODELS_IN_CACHE="+strconv.Itoa(v))
	}
	if v, _ := cmd.Flags().GetString("cache-ttl"); v != "" {
		envVars = append(envVars, "KRONK_CACHE_TTL="+v)
	}
	if cmd.Flags().Changed("ignore-integrity-check") {
		v, _ := cmd.Flags().GetBool("ignore-integrity-check")
		envVars = append(envVars, "KRONK_CACHE_IGNORE_INTEGRITY_CHECK="+strconv.FormatBool(v))
	}

	// Runtime settings
	if v, _ := cmd.Flags().GetString("base-path"); v != "" {
		envVars = append(envVars, "KRONK_BASE_PATH="+v)
	}
	if v, _ := cmd.Flags().GetString("lib-path"); v != "" {
		envVars = append(envVars, "KRONK_LIB_PATH="+v)
	}
	if v, _ := cmd.Flags().GetString("lib-version"); v != "" {
		envVars = append(envVars, "KRONK_LIB_VERSION="+v)
	}
	if v, _ := cmd.Flags().GetString("arch"); v != "" {
		envVars = append(envVars, "KRONK_ARCH="+v)
	}
	if v, _ := cmd.Flags().GetString("os"); v != "" {
		envVars = append(envVars, "KRONK_OS="+v)
	}
	if v, _ := cmd.Flags().GetString("processor"); v != "" {
		envVars = append(envVars, "KRONK_PROCESSOR="+v)
	}
	if v, _ := cmd.Flags().GetString("hf-token"); v != "" {
		envVars = append(envVars, "KRONK_HF_TOKEN="+v)
	}
	if cmd.Flags().Changed("allow-upgrade") {
		v, _ := cmd.Flags().GetBool("allow-upgrade")
		envVars = append(envVars, "KRONK_ALLOW_UPGRADE="+strconv.FormatBool(v))
	}
	if v, _ := cmd.Flags().GetInt("llama-log"); v != -1 {
		envVars = append(envVars, "KRONK_LLAMA_LOG="+strconv.Itoa(v))
	}
	if cmd.Flags().Changed("insecure-logging") {
		v, _ := cmd.Flags().GetBool("insecure-logging")
		envVars = append(envVars, "KRONK_INSECURE_LOGGING="+strconv.FormatBool(v))
	}

	return envVars
}

func splitEnvVar(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}

func logFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.log")
}

func pidFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.pid")
}
