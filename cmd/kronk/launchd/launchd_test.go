package launchd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("non-macOS behavior")
	}
	cmd := newCmd()
	cmd.SetArgs([]string{"install"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "only on macOS") {
		t.Fatalf("install: got %v, want unsupported-platform error", err)
	}
}

func TestAgentLifecycle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("LaunchAgent installation is macOS-only")
	}
	home := filepath.Join(t.TempDir(), `home space & <xml> "quoted"`)
	t.Setenv("HOME", home)
	t.Setenv("KRONK_BASE_PATH", "/ignored-environment-path")
	t.Setenv("AGENT_STATE", filepath.Join(t.TempDir(), "loaded.plist"))
	t.Setenv("AGENT_FAIL", "")
	binDir := t.TempDir()
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	// Model launchd's loaded definition separately from the on-disk plist, so
	// a restart that fails to reload edited configuration is observable.
	script := `#!/bin/sh
if [ -n "$AGENT_FAIL" ]; then echo "GUI domain unavailable" >&2; exit 112; fi
case "$1" in
  print)
    if [ -f "$AGENT_STATE.stopping" ]; then cat "$AGENT_STATE.stopping"; rm "$AGENT_STATE.stopping"
    elif [ -f "$AGENT_STATE" ]; then cat "$AGENT_STATE"
    else exit 113; fi ;;
  bootstrap) cp "$3" "$AGENT_STATE" ;;
  bootout) mv "$AGENT_STATE" "$AGENT_STATE.stopping" ;;
  kickstart|enable|disable) ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "launchctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		root := &cobra.Command{Use: "kronk", SilenceErrors: true, SilenceUsage: true}
		root.PersistentFlags().String("base-path", "", "")
		root.AddCommand(newCmd())
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{"launchd"}, args...))
		err := root.Execute()
		return out.String(), err
	}

	basePath := filepath.Join(home, `models & <data> "quoted"`)
	if out, err := run("install", "--base-path", basePath); err != nil {
		t.Fatalf("install: %v: %s", err, out)
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ardanlabs.kronk.plist")
	logPath := filepath.Join(home, "Library", "Logs", "kronk", "kronk.log")
	jsonData, err := exec.Command("plutil", "-convert", "json", "-o", "-", plistPath).Output()
	if err != nil {
		t.Fatalf("native plist validation: %v", err)
	}
	var agent struct {
		Label                string
		ProgramArguments     []string
		EnvironmentVariables map[string]string
		WorkingDirectory     string
		StandardOutPath      string
		StandardErrorPath    string
		RunAtLoad            bool
		KeepAlive            bool
		ExitTimeOut          int
	}
	if err := json.Unmarshal(jsonData, &agent); err != nil {
		t.Fatal(err)
	}
	if agent.Label != "com.ardanlabs.kronk" || len(agent.ProgramArguments) != 3 ||
		!filepath.IsAbs(agent.ProgramArguments[0]) || !slices.Equal(agent.ProgramArguments[1:], []string{"server", "start"}) {
		t.Fatalf("installed agent: got %+v, want absolute executable with server start", agent)
	}
	if agent.EnvironmentVariables["KRONK_BASE_PATH"] != basePath || agent.EnvironmentVariables["HOME"] != home || agent.WorkingDirectory != home {
		t.Fatalf("installed paths did not survive XML escaping: %+v", agent)
	}
	if agent.StandardOutPath != logPath || agent.StandardErrorPath != logPath || !agent.RunAtLoad || !agent.KeepAlive || agent.ExitTimeOut < 150 {
		t.Fatalf("installed lifecycle/logging configuration: got %+v", agent)
	}
	if _, err := run("status"); err == nil {
		t.Fatal("status after install: got nil error, want not loaded")
	}
	plist, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run("install"); err == nil {
		t.Fatal("second install: got nil error, want refusal to overwrite")
	}
	if got, err := os.ReadFile(plistPath); err != nil || !bytes.Equal(got, plist) {
		t.Fatalf("second install changed existing plist: %s, %v", got, err)
	}
	if out, err := run("start"); err != nil {
		t.Fatalf("start: %v: %s", err, out)
	}
	if out, err := run("start"); err != nil {
		t.Fatalf("already-started agent: %v: %s", err, out)
	}
	if out, err := exec.Command("plutil", "-insert", "EnvironmentVariables.KRONK_WEB_API_HOST", "-string", "127.0.0.1:22567", plistPath).CombinedOutput(); err != nil {
		t.Fatalf("editing configuration: %v: %s", err, out)
	}
	if out, err := run("restart"); err != nil {
		t.Fatalf("restart: %v: %s", err, out)
	}
	if out, err := run("status"); err != nil || !strings.Contains(out, "127.0.0.1:22567") {
		t.Fatalf("restarted configuration: got %q, %v, want updated listener", out, err)
	}
	for range 2 {
		if out, err := run("stop"); err != nil {
			t.Fatalf("stop: %v: %s", err, out)
		}
	}
	if _, err := run("status"); err == nil {
		t.Fatal("stopped status: got nil error, want not loaded")
	}
	if out, err := run("restart"); err != nil {
		t.Fatalf("restart stopped agent: %v: %s", err, out)
	}
	t.Setenv("AGENT_FAIL", "1")
	if _, err := run("uninstall"); err == nil {
		t.Fatal("uninstall with unavailable GUI domain: got nil error")
	}
	if _, err := os.Stat(plistPath); err != nil {
		t.Fatalf("failed uninstall removed plist: %v", err)
	}
	t.Setenv("AGENT_FAIL", "")
	logData := []byte("retained service log\n")
	if err := os.WriteFile(logPath, logData, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("install", "--force"); err != nil {
		t.Fatalf("force reinstall: %v: %s", err, out)
	}
	if out, err := run("uninstall"); err != nil {
		t.Fatalf("uninstall: %v: %s", err, out)
	}
	if _, err := os.Stat(plistPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uninstalled plist: got %v, want not found", err)
	}
	if got, err := os.ReadFile(logPath); err != nil || !bytes.Equal(got, logData) {
		t.Fatalf("logs after reinstall/uninstall: got %q, %v, want preserved", got, err)
	}
}
