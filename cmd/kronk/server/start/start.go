// Package start manages the server start sub-command.
package start

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

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

		proc := exec.Command(exePath, "server")
		proc.Stdout = logFile
		proc.Stderr = logFile
		proc.Stdin = nil
		proc.Env = append(os.Environ(), envVars...)
		proc.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}

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

	if v, _ := cmd.Flags().GetString("api-host"); v != "" {
		envVars = append(envVars, "KRONK_WEB_API_HOST="+v)
	}

	if v, _ := cmd.Flags().GetString("debug-host"); v != "" {
		envVars = append(envVars, "KRONK_WEB_DEBUG_HOST="+v)
	}

	if cmd.Flags().Changed("auth-enabled") {
		v, _ := cmd.Flags().GetBool("auth-enabled")
		envVars = append(envVars, "KRONK_AUTH_LOCAL_ENABLED="+strconv.FormatBool(v))
	}

	if v, _ := cmd.Flags().GetInt("model-instances"); v != 0 {
		envVars = append(envVars, "KRONK_CACHE_MODEL_INSTANCES="+strconv.Itoa(v))
	}

	if v, _ := cmd.Flags().GetInt("models-in-cache"); v != 0 {
		envVars = append(envVars, "KRONK_CACHE_MODELS-IN-CACHE="+strconv.Itoa(v))
	}

	if v, _ := cmd.Flags().GetString("cache-ttl"); v != "" {
		envVars = append(envVars, "KRONK_CACHE_TTL="+v)
	}

	if v, _ := cmd.Flags().GetBool("ignore-integrity-check"); v {
		envVars = append(envVars, "KRONK_CACHE_IGNORE_INTEGRITY_CHECK=true")
	}

	if v, _ := cmd.Flags().GetString("model-config-file"); v != "" {
		envVars = append(envVars, "KRONK_MODEL_CONFIG_FILE="+v)
	}

	if v, _ := cmd.Flags().GetInt("llama-log"); v != -1 {
		envVars = append(envVars, "KRONK_LLAMA_LOG="+strconv.Itoa(v))
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
