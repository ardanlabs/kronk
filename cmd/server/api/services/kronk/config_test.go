package kronk

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"go.yaml.in/yaml/v2"
)

func TestLoadConfig(t *testing.T) {
	unsetEnv(t, "KRONK_HF_TOKEN")
	unsetEnv(t, "KRONK_LLAMA_LOG")

	path := filepath.Join(t.TempDir(), "model_config.yaml")
	data := []byte(`version: 1
models: {}
kms:
  web:
    api-host: yaml.example:9000
    read-timeout: 12s
  authorization:
    mode: authenticated
  mcp:
    enabled: false
  pool:
    budget-percent: 80
    ttl: 5m
  bucky-lib-path: /yaml/bucky
  hf-token: yaml-token
  llama-log: 0
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("KRONK_POOL_MODEL_CONFIG_FILE", path)
	t.Setenv("KRONK_WEB_API_HOST", "env.example:9001")
	t.Setenv("KRONK_POOL_BUDGET_PERCENT", "75")

	cfg, err := loadConfig(false)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if cfg.Web.APIHost != "env.example:9001" {
		t.Errorf("APIHost: got %q, want %q", cfg.Web.APIHost, "env.example:9001")
	}
	if cfg.Web.ReadTimeout != 12*time.Second {
		t.Errorf("ReadTimeout: got %s, want %s", cfg.Web.ReadTimeout, 12*time.Second)
	}
	if cfg.MCP.Enabled {
		t.Error("MCP.Enabled: got true, want false")
	}
	if cfg.Pool.BudgetPercent != 75 {
		t.Errorf("BudgetPercent: got %d, want %d", cfg.Pool.BudgetPercent, 75)
	}
	if cfg.Pool.TTL != 5*time.Minute {
		t.Errorf("TTL: got %s, want %s", cfg.Pool.TTL, 5*time.Minute)
	}
	if cfg.BuckyLibPath != "/yaml/bucky" {
		t.Errorf("BuckyLibPath: got %q, want %q", cfg.BuckyLibPath, "/yaml/bucky")
	}
	if token := os.Getenv("KRONK_HF_TOKEN"); token != "yaml-token" {
		t.Errorf("KRONK_HF_TOKEN: got %q, want %q", token, "yaml-token")
	}
	if level := os.Getenv("KRONK_LLAMA_LOG"); level != "0" {
		t.Errorf("KRONK_LLAMA_LOG: got %q, want %q", level, "0")
	}
	if cfg.Authorization.Mode.String() != "authenticated" {
		t.Errorf("Authorization.Mode: got %q, want %q", cfg.Authorization.Mode, "authenticated")
	}
	if cfg.Pool.ModelConfigFile != path {
		t.Errorf("ModelConfigFile: got %q, want %q", cfg.Pool.ModelConfigFile, path)
	}
}

func TestLoadConfigVersionZero(t *testing.T) {
	unsetEnv(t, "KRONK_HF_TOKEN")
	unsetEnv(t, "KRONK_LLAMA_LOG")

	path := filepath.Join(t.TempDir(), "model_config.yaml")
	data := []byte("owner/model:\n  context-window: 4096\nkms:\n  web:\n    api-host: ignored:9000\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("KRONK_POOL_MODEL_CONFIG_FILE", path)

	cfg, err := loadConfig(false)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Web.APIHost != "127.0.0.1:11435" {
		t.Errorf("APIHost: got %q, want default", cfg.Web.APIHost)
	}

	upgraded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var doc struct {
		Version int            `yaml:"version"`
		Models  map[string]any `yaml:"models"`
	}
	if err := yaml.Unmarshal(upgraded, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if doc.Version != configVersion {
		t.Errorf("Version: got %d, want %d", doc.Version, configVersion)
	}
	if _, exists := doc.Models["owner/model"]; !exists {
		t.Errorf("Models: got %v, want owner/model", doc.Models)
	}
}

func TestLoadConfigUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model_config.yaml")
	if err := os.WriteFile(path, []byte("version: 2\nmodels: {}\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("KRONK_POOL_MODEL_CONFIG_FILE", path)

	if _, err := loadConfig(false); err == nil {
		t.Fatal("loadConfig: got nil error, want unsupported version error")
	}
}

func TestResolveAuthorizationSettings(t *testing.T) {
	tests := []struct {
		name                 string
		mode                 auth.Mode
		legacyInference      bool
		legacyManagement     bool
		mcp                  bool
		wantInference        bool
		wantManagement       bool
		wantServiceAdminAuth bool
	}{
		{name: "legacy open"},
		{name: "legacy management", legacyManagement: true, wantManagement: true, wantServiceAdminAuth: true},
		{name: "legacy inference implies management", legacyInference: true, wantInference: true, wantManagement: true, wantServiceAdminAuth: true},
		{name: "legacy MCP implies management", mcp: true, wantManagement: true, wantServiceAdminAuth: true},
		{name: "open overrides legacy", mode: auth.Open, legacyInference: true, legacyManagement: true},
		{name: "open preserves MCP auth service", mode: auth.Open, mcp: true, wantServiceAdminAuth: true},
		{name: "management overrides legacy", mode: auth.Management, legacyInference: true, wantManagement: true, wantServiceAdminAuth: true},
		{name: "authenticated", mode: auth.Authenticated, wantInference: true, wantManagement: true, wantServiceAdminAuth: true},
		{name: "full protected", mode: auth.FullProtected, wantInference: true, wantManagement: true, wantServiceAdminAuth: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotInference, gotManagement, gotServiceAdminAuth := resolveAuthorizationSettings(tt.mode, tt.legacyInference, tt.legacyManagement, tt.mcp)
			if gotInference != tt.wantInference {
				t.Errorf("inference enabled: got %t, want %t", gotInference, tt.wantInference)
			}
			if gotManagement != tt.wantManagement {
				t.Errorf("management enabled: got %t, want %t", gotManagement, tt.wantManagement)
			}
			if gotServiceAdminAuth != tt.wantServiceAdminAuth {
				t.Errorf("service admin auth enabled: got %t, want %t", gotServiceAdminAuth, tt.wantServiceAdminAuth)
			}
		})
	}
}

func TestValidateTimeoutConfig(t *testing.T) {
	tests := []struct {
		name      string
		inference time.Duration
		write     time.Duration
		wantErr   bool
	}{
		{name: "defaults", inference: 60 * time.Minute, write: 61 * time.Minute},
		{name: "write disabled", inference: time.Minute},
		{name: "inference zero", write: time.Minute, wantErr: true},
		{name: "inference negative", inference: -time.Second, write: time.Minute, wantErr: true},
		{name: "write equal", inference: time.Minute, write: time.Minute, wantErr: true},
		{name: "write shorter", inference: time.Minute, write: time.Second, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimeoutConfig(tt.inference, tt.write)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateTimeoutConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAdminConfig(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		name     string
		admin    bool
		web      bool
		password string
		host     string
		wantErr  bool
	}{
		{name: "open"},
		{name: "admin only", admin: true},
		{name: "open web admin", web: true},
		{name: "protected web admin", admin: true, web: true, password: sha},
		{name: "protected web missing password", admin: true, web: true, wantErr: true},
		{name: "inactive password", password: sha},
		{name: "inactive web password with external auth", web: true, password: sha, host: "auth:9000"},
		{name: "short digest", admin: true, password: "abcd", wantErr: true},
		{name: "non hex digest", admin: true, password: sha[:63] + "z", wantErr: true},
		{name: "external browser login", admin: true, web: true, password: sha, host: "auth:9000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAdminConfig(tt.admin, tt.web, tt.password, tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAdminConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()

	value, exists := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv %s: %v", name, err)
	}
	t.Cleanup(func() {
		if exists {
			if err := os.Setenv(name, value); err != nil {
				t.Errorf("Setenv %s: %v", name, err)
			}
			return
		}
		if err := os.Unsetenv(name); err != nil {
			t.Errorf("Unsetenv %s: %v", name, err)
		}
	})
}
