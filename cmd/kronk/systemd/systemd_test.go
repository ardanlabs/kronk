package systemd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux behavior")
	}
	cmd := newCmd()
	cmd.SetArgs([]string{"install"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "only on Linux") {
		t.Fatalf("install: got %v, want unsupported-platform error", err)
	}
}

func TestServiceLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd installation is Linux-only")
	}
	configDir := filepath.Join(t.TempDir(), "config space %f $HOME")
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("KRONK_BASE_PATH", "")
	binDir := t.TempDir()
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("SERVICE_STATE", filepath.Join(t.TempDir(), "state"))
	t.Setenv("SERVICE_FAIL", "")
	t.Setenv("LINGER_FAIL", "")
	// A local service-manager stand-in exercises the CLI without changing the
	// host's services. Actual unit syntax is additionally checked on Linux.
	script := `#!/bin/sh
if [ -n "$SERVICE_FAIL" ]; then echo "manager unavailable" >&2; exit 1; fi
case "$2" in
  start|restart) echo active > "$SERVICE_STATE" ;;
  stop|disable) rm -f "$SERVICE_STATE" ;;
  status) if [ -f "$SERVICE_STATE" ]; then echo active; else echo inactive; exit 3; fi ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	lingerScript := "#!/bin/sh\nif [ -n \"$LINGER_FAIL\" ]; then exit 1; fi\necho enabled > \"$SERVICE_STATE.linger\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "loginctl"), []byte(lingerScript), 0o755); err != nil {
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
		root.SetArgs(append([]string{"systemd"}, args...))
		err := root.Execute()
		return out.String(), err
	}

	basePath := filepath.Join(t.TempDir(), "models space %n $HOME")
	if out, err := run("install", "--base-path", basePath); err != nil {
		t.Fatalf("install: %v: %s", err, out)
	}
	unitPath := filepath.Join(configDir, "systemd", "user", "kronk.service")
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		` server start`,
		`Environment="KRONK_BASE_PATH=` + strings.ReplaceAll(basePath, "%", "%%") + `"`,
		"EnvironmentFile=-" + strings.ReplaceAll(configDir, "%", "%%") + "/kronk/kronk.env\n",
	} {
		if !strings.Contains(string(unit), want) {
			t.Errorf("installed unit: got %s, want %q", unit, want)
		}
	}
	if strings.Contains(string(unit), "--detach") || strings.Contains(string(unit), "@EXECUTABLE@") {
		t.Fatalf("installed unit has unresolved template or detached process: %s", unit)
	}
	if out, err := run("status"); err == nil || !strings.Contains(out, "inactive") {
		t.Fatalf("status after install: got %q, %v, want inactive error", out, err)
	}
	if _, err := run("install"); err == nil {
		t.Fatal("second install: got nil error, want refusal to overwrite")
	}
	if got, err := os.ReadFile(unitPath); err != nil || !bytes.Equal(got, unit) {
		t.Fatalf("second install changed existing unit: %s, %v", got, err)
	}
	for _, action := range []string{"start", "restart"} {
		if out, err := run(action); err != nil {
			t.Fatalf("%s: %v: %s", action, err, out)
		}
		if out, err := run("status"); err != nil || strings.TrimSpace(out) != "active" {
			t.Fatalf("status: got %q, %v, want active", out, err)
		}
	}
	if out, err := run("stop"); err != nil {
		t.Fatalf("stop: %v: %s", err, out)
	}
	if _, err := run("status"); err == nil {
		t.Fatal("stopped status: got nil error, want nonzero exit")
	}

	envPath := filepath.Join(configDir, "kronk", "kronk.env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatal(err)
	}
	settings := []byte("KRONK_PROCESSOR=vulkan\n")
	if err := os.WriteFile(envPath, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("install", "--force"); err != nil {
		t.Fatalf("reinstall: %v: %s", err, out)
	}
	t.Setenv("LINGER_FAIL", "1")
	if _, err := run("install", "--force", "--boot"); err == nil || !strings.Contains(err.Error(), "service installed, but") {
		t.Fatalf("boot authorization failure: got %v, want partial-install explanation", err)
	}
	t.Setenv("LINGER_FAIL", "")
	if out, err := run("install", "--force", "--boot"); err != nil {
		t.Fatalf("boot install: %v: %s", err, out)
	}
	if got, err := os.ReadFile(os.Getenv("SERVICE_STATE") + ".linger"); err != nil || string(got) != "enabled\n" {
		t.Fatalf("boot startup: got %q, %v, want lingering enabled", got, err)
	}
	t.Setenv("SERVICE_FAIL", "1")
	if _, err := run("uninstall"); err == nil {
		t.Fatal("uninstall with unavailable manager: got nil error")
	}
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("failed uninstall removed unit: %v", err)
	}
	t.Setenv("SERVICE_FAIL", "")
	if out, err := run("uninstall"); err != nil {
		t.Fatalf("uninstall: %v: %s", err, out)
	}
	if _, err := os.Stat(unitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uninstalled unit: got %v, want not found", err)
	}
	if got, err := os.ReadFile(envPath); err != nil || !bytes.Equal(got, settings) {
		t.Fatalf("settings after uninstall: got %q, %v, want preserved", got, err)
	}

	// Verify the original process error remains available to the CLI's exit-code
	// handling, rather than turning an inactive service into a successful status.
	_, err = run("status")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("inactive status: got %v, want exit code 3", err)
	}
}
