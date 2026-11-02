package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	"golang.org/x/term"
)

const serviceName = "kronk"

func installService(cmd *cobra.Command, args []string, server *cobra.Command) error {
	if err := requireElevation(); err != nil {
		return err
	}
	startType, _ := cmd.Flags().GetString("start-type")
	types := map[string]uint32{"auto": windows.SERVICE_AUTO_START, "manual": windows.SERVICE_DEMAND_START}
	startup, ok := types[startType]
	if !ok {
		return errors.New("start-type must be auto or manual")
	}
	for _, flag := range []string{"lib-path", "bucky-lib-path", "malina-lib-path", "model-config-file", "auth-tls-ca-file"} {
		value, _ := server.Flags().GetString(flag)
		if value != "" && !filepath.IsAbs(value) {
			return fmt.Errorf("server flag --%s requires an absolute path for a service", flag)
		}
	}
	basePath := client.GetBasePath(cmd)
	if basePath == "" {
		programData := os.Getenv("ProgramData")
		if !filepath.IsAbs(programData) {
			return errors.New("ProgramData is unavailable; supply an absolute --base-path")
		}
		basePath = filepath.Join(programData, "Kronk")
	}
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("resolving base path: %w", err)
	}
	if filepath.Dir(basePath) == basePath {
		return errors.New("base path must be a dedicated data directory, not a drive root")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}
	if strings.Contains(strings.ToLower(executable), "go-build") {
		return errors.New("install from a permanent kronk.exe, not go run")
	}
	m, err := mgr.Connect()
	if err != nil {
		return accessError(err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return accessError(err)
	}
	exists := err == nil
	account, _ := cmd.Flags().GetString("account")
	if exists {
		defer s.Close()
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			return errors.New("service already exists; stop it and use install --force to update")
		}
		state, err := s.Query()
		if err != nil {
			return accessError(err)
		}
		if state.State != svc.Stopped {
			return errors.New("stop the service before replacing its registration")
		}
		if !cmd.Flags().Changed("account") {
			cfg, err := s.Config()
			if err != nil {
				return accessError(err)
			}
			account = cfg.ServiceStartName
		}
	}
	if strings.TrimSpace(account) == "" {
		return errors.New("account must not be empty")
	}
	var password string
	if !exists || cmd.Flags().Changed("account") {
		switch strings.ToLower(account) {
		case "localsystem", `nt authority\localservice`, `nt authority\networkservice`:
		default:
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("a terminal is required to securely read the service account password")
			}
			cmd.Printf("Password for %s: ", account)
			data, err := term.ReadPassword(int(os.Stdin.Fd()))
			cmd.Println()
			if err != nil {
				return fmt.Errorf("reading account password: %w", err)
			}
			password = string(data)
			clear(data)
		}
	}
	if err := protectDataDirectory(basePath, account); err != nil {
		return err
	}
	serviceArgs := append([]string{"server", "start", "--base-path", basePath}, args...)
	cfg := mgr.Config{
		DisplayName: "Kronk Model Server", Description: "Local model inference server",
		ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, ErrorControl: windows.SERVICE_ERROR_NORMAL,
		StartType: startup, ServiceStartName: account, Password: password,
	}
	if exists {
		cfg.BinaryPathName = windows.ComposeCommandLine(append([]string{executable}, serviceArgs...))
		// An empty account leaves both the account and its stored password unchanged.
		if !cmd.Flags().Changed("account") {
			cfg.ServiceStartName = ""
		}
		if err := s.UpdateConfig(cfg); err != nil {
			return accessError(err)
		}
	} else {
		s, err = m.CreateService(serviceName, executable, cfg, serviceArgs...)
		if err != nil {
			return accessError(err)
		}
		defer s.Close()
	}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.NoAction},
	}, 86400); err != nil {
		return fmt.Errorf("service registered, but configuring recovery failed: %w", err)
	}
	if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("service registered, but enabling failure recovery failed: %w", err)
	}
	// Recreate the Event Log source when explicitly reinstalling. Server data is untouched.
	if exists {
		if err := eventlog.Remove(serviceName); err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("service registered, but replacing Event Log source failed: %w", err)
		}
	}
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		return fmt.Errorf("service registered, but installing Event Log source failed: %w", err)
	}
	cmd.Printf("Service registered but not started. Data: %s\nRun kronk service start, then check /v1/liveness.\n", basePath)
	return nil
}

func protectDataDirectory(path, account string) error {
	if strings.EqualFold(account, "LocalSystem") {
		account = `NT AUTHORITY\SYSTEM`
	}
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil {
		return fmt.Errorf("resolving service account: %w", err)
	}
	// Reject junctions and symlinks instead of changing permissions on their targets.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		name, err := windows.UTF16PtrFromString(parent)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil {
			return fmt.Errorf("checking service data parent (create parent directories first): %w", err)
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("service data parent must not be a reparse point: %s", parent)
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	// Create atomically with an administrator owner and protected, inheritable ACL.
	// Existing trees are validated, never silently re-permissioned or migrated.
	sd, err := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + sid.String() + ")")
	if err != nil {
		return fmt.Errorf("creating service data ACL: %w", err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes := windows.SecurityAttributes{SecurityDescriptor: sd}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	if err := windows.CreateDirectory(name, &attributes); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fmt.Errorf("creating protected service data directory: %w", err)
	}
	return filepath.WalkDir(path, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil {
			return err
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("service data must not contain reparse points: %s", path)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return fmt.Errorf("checking permissions for %s: %w", path, err)
		}
		if err := validateDataSecurity(sd, sid.String()); err != nil {
			return fmt.Errorf("unsafe service data permissions for %s; use a new dedicated directory or have an administrator secure the existing tree: %w", path, err)
		}
		return nil
	})
}

func validateDataSecurity(sd *windows.SECURITY_DESCRIPTOR, accountSID string) error {
	// FILE_ALL_ACCESS from WinNT.h: standard rights plus all nine file rights.
	const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	if sd == nil {
		return errors.New("missing security descriptor")
	}
	trusted := func(sid string) bool {
		return sid == "S-1-5-18" || sid == "S-1-5-32-544" || sid == accountSID
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trusted(owner.String()) {
		return errors.New("owner must be Administrators, SYSTEM, or the service account")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return errors.New("missing discretionary access control list")
	}
	accountAccess := false
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("reading data access rule: %w", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("only ordinary allow rules are supported in service data ACLs")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if !trusted(sid) {
			return errors.New("data access is granted to an account other than Administrators, SYSTEM, or the service account")
		}
		if sid == accountSID && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 && (ace.Mask&fileAllAccess == fileAllAccess || ace.Mask&windows.GENERIC_ALL != 0) {
			accountAccess = true
		}
	}
	if !accountAccess {
		return errors.New("service account requires full control of service data")
	}
	return nil
}

func requireElevation() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("service management requires elevation; open PowerShell or Windows Terminal with Run as administrator")
	}
	return nil
}

func accessError(err error) error {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("access denied; open PowerShell or Windows Terminal with Run as administrator: %w", err)
	}
	return fmt.Errorf("windows service manager: %w", err)
}

func manageService(cmd *cobra.Command, action string) error {
	if action != "status" && action != "logs" {
		if err := requireElevation(); err != nil {
			return err
		}
	}
	// Connect/OpenService request administrative access. Status and logs use
	// read-only handles so Windows can grant them independently of elevation.
	access := uint32(windows.SC_MANAGER_CONNECT)
	handle, err := windows.OpenSCManager(nil, nil, access)
	if err != nil {
		return accessError(err)
	}
	m := mgr.Mgr{Handle: handle}
	defer m.Disconnect()
	name, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return err
	}
	rights := uint32(windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG)
	if action != "status" && action != "logs" {
		rights = windows.SERVICE_ALL_ACCESS
	}
	serviceHandle, err := windows.OpenService(m.Handle, name, rights)
	if err != nil {
		return accessError(err)
	}
	s := mgr.Service{Name: serviceName, Handle: serviceHandle}
	defer s.Close()
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer cancel()
	if action == "logs" {
		cfg, err := s.Config()
		if err != nil {
			return accessError(err)
		}
		args, err := windows.DecomposeCommandLine(cfg.BinaryPathName)
		if err != nil {
			return err
		}
		for i, arg := range args {
			if arg == "--base-path" && i+1 < len(args) {
				return followLog(ctx, filepath.Join(args[i+1], "service.log"), cmd.OutOrStdout())
			}
		}
		return errors.New("installed service has no base path; reinstall it")
	}
	if action == "status" {
		state, err := s.Query()
		if err != nil {
			return accessError(err)
		}
		states := map[svc.State]string{svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping", svc.Running: "running", svc.Paused: "paused"}
		cmd.Printf("Kronk: %s (state=%d, PID=%d, exit=%d, service-exit=%d)\n", states[state.State], state.State, state.ProcessId, state.Win32ExitCode, state.ServiceSpecificExitCode)
		return nil
	}
	ctx, timeout := context.WithTimeout(ctx, 5*time.Minute)
	defer timeout()
	if action == "stop" || action == "restart" || action == "uninstall" {
		if err := stopService(ctx, &s); err != nil {
			return err
		}
	}
	if action == "uninstall" {
		if err := s.Delete(); err != nil {
			return accessError(err)
		}
		if err := eventlog.Remove(serviceName); err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("service removed, but removing Event Log source failed: %w", err)
		}
		cmd.Println("Service removed; data, settings, and logs preserved.")
		return nil
	}
	if action == "start" || action == "restart" {
		state, err := s.Query()
		if err != nil {
			return accessError(err)
		}
		if state.State == svc.StopPending {
			if err := waitState(ctx, &s, svc.Stopped); err != nil {
				return err
			}
			state.State = svc.Stopped
		}
		if state.State == svc.Stopped {
			if err := s.Start(); err != nil {
				return accessError(err)
			}
		}
		if err := waitState(ctx, &s, svc.Running); err != nil {
			return err
		}
		cmd.Println("Service running; check /v1/liveness for HTTP readiness.")
	}
	return nil
}

func stopService(ctx context.Context, s *mgr.Service) error {
	state, err := s.Query()
	if err != nil {
		return accessError(err)
	}
	if state.State == svc.Stopped {
		return nil
	}
	if state.State != svc.StopPending {
		if _, err := s.Control(svc.Stop); err != nil {
			return accessError(err)
		}
	}
	return waitState(ctx, s, svc.Stopped)
}

func waitState(ctx context.Context, s *mgr.Service, want svc.State) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := s.Query()
		if err != nil {
			return accessError(err)
		}
		if state.State == want {
			return nil
		}
		if want == svc.Running && state.State == svc.Stopped {
			return fmt.Errorf("service stopped during startup (exit=%d, service-exit=%d); inspect service logs and Event Viewer", state.Win32ExitCode, state.ServiceSpecificExitCode)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for service state %d (the service may still be working): %w", want, ctx.Err())
		case <-ticker.C:
		}
	}
}

func followLog(ctx context.Context, path string, output io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening service log (start the service first): %w", err)
	}
	defer file.Close()
	// Start near the end, but continue following bytes appended by the service.
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 64*1024 {
		if _, err := file.Seek(-64*1024, io.SeekEnd); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := io.Copy(output, file); err != nil {
			return fmt.Errorf("reading service log: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
