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
		proc.Env = overrideEnv(os.Environ(), envVars)
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
	if cmd.Flags().Changed("auth-enabled") {
		v, _ := cmd.Flags().GetBool("auth-enabled")
		envVars = append(envVars, "KRONK_AUTH_LOCAL_ENABLED="+strconv.FormatBool(v))
		if v {
			envVars = append(envVars, "KRONK_AUTH_ADMIN_ENABLED=true")
		}
	}
	if cmd.Flags().Changed("admin-auth-enabled") {
		v, _ := cmd.Flags().GetBool("admin-auth-enabled")
		general, _ := cmd.Flags().GetBool("auth-enabled")
		if !general {
			envVars = append(envVars, "KRONK_AUTH_ADMIN_ENABLED="+strconv.FormatBool(v))
		}
	}
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
	addString("lib-version", "KRONK_LIB_VERSION")
	addBool("lib-download-enabled", "KRONK_LIB_DOWNLOAD_ENABLED")
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

func logFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.log")
}

func pidFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.pid")
}
