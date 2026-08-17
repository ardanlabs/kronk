package kronk

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"go.yaml.in/yaml/v2"
)

const configVersion = 1

type config struct {
	conf.Version `yaml:"-"`
	Web          struct {
		ReadTimeout        time.Duration `yaml:"read-timeout"`
		WriteTimeout       time.Duration `yaml:"write-timeout"`
		InferenceTimeout   time.Duration `yaml:"inference-timeout"`
		IdleTimeout        time.Duration `yaml:"idle-timeout"`
		ShutdownTimeout    time.Duration `yaml:"shutdown-timeout"`
		APIHost            string        `yaml:"api-host"`
		DebugHost          string        `yaml:"debug-host"`
		CORSAllowedOrigins []string      `yaml:"cors-allowed-origins"`
		Admin              struct {
			Enabled        bool   `yaml:"enabled"`
			PasswordSHA256 string `conf:"mask" yaml:"password-sha-256"`
		} `yaml:"admin"`
	} `yaml:"web"`
	Auth struct {
		Host         string `yaml:"host"`
		AdminEnabled bool   `yaml:"admin-enabled"`
		TLS          struct {
			Enabled    bool   `yaml:"enabled"`
			CAFile     string `yaml:"ca-file"`
			ServerName string `yaml:"server-name"`
		} `yaml:"tls"`
		Local struct {
			Issuer  string `yaml:"issuer"`
			Enabled bool   `yaml:"enabled"`
		} `yaml:"local"`
	} `yaml:"auth"`
	Authorization struct {
		Mode auth.Mode `yaml:"mode"`
	} `yaml:"authorization"`
	MCP struct {
		Enabled     bool   `yaml:"enabled"`
		Host        string `yaml:"host"`
		AuthEnabled bool   `yaml:"auth-enabled"`
		BraveAPIKey string `conf:"mask" yaml:"brave-api-key"`
	} `yaml:"mcp"`
	Download struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"download"`
	Tempo struct {
		Host        string  `yaml:"host"`
		ServiceName string  `yaml:"service-name"`
		Probability float64 `yaml:"probability"`
	} `yaml:"tempo"`
	Pool struct {
		ModelConfigFile string        `yaml:"-"`
		BudgetPercent   int           `yaml:"budget-percent"`
		ModelsInPool    int           `yaml:"models-in-pool"`
		TTL             time.Duration `yaml:"ttl"`
	} `yaml:"pool"`
	BasePath        string `yaml:"base-path"`
	LibPath         string `yaml:"lib-path"`
	BuckyLibPath    string `yaml:"bucky-lib-path"`
	LibVersion      string `yaml:"lib-version"`
	Arch            string `yaml:"arch"`
	OS              string `yaml:"os"`
	Processor       string `yaml:"processor"`
	AllowUpgrade    bool   `yaml:"allow-upgrade"`
	InsecureLogging bool   `yaml:"insecure-logging"`
	HfToken         string `conf:"mask" yaml:"hf-token"`
	LlamaLog        int    `yaml:"llama-log"`
}

func newConfig() config {
	cfg := config{
		Version: conf.Version{
			Build: tag,
			Desc:  "Kronk",
		},
		LlamaLog: 1,
	}

	cfg.Web.ReadTimeout = 30 * time.Second
	cfg.Web.WriteTimeout = 61 * time.Minute
	cfg.Web.InferenceTimeout = 60 * time.Minute
	cfg.Web.IdleTimeout = time.Minute
	cfg.Web.ShutdownTimeout = time.Minute
	cfg.Web.APIHost = "127.0.0.1:11435"
	cfg.Web.DebugHost = "127.0.0.1:11445"
	cfg.Web.CORSAllowedOrigins = []string{"*"}
	cfg.Web.Admin.Enabled = true
	cfg.Web.Admin.PasswordSHA256 = "18511e63760230cd17291273b607e7e13da2a2bb9a1750e0becdac08185a3c11"
	cfg.Auth.Local.Issuer = "kronk project"
	cfg.MCP.Enabled = true
	cfg.Tempo.Host = "localhost:4317"
	cfg.Tempo.ServiceName = "kronk"
	cfg.Tempo.Probability = 0.25
	cfg.Pool.BudgetPercent = 95
	cfg.Pool.ModelsInPool = 10

	return cfg
}

func loadConfig(showHelp bool) (config, error) {
	cfg := newConfig()

	const prefix = "KRONK"
	if showHelp {
		help, err := conf.UsageInfo(prefix, &cfg)
		if err != nil {
			return config{}, fmt.Errorf("parsing config: %w", err)
		}
		return config{}, fmt.Errorf("%s", help)
	}

	var location struct {
		Pool struct {
			ModelConfigFile string
		}
		BasePath string
	}
	if _, err := conf.Parse(prefix, &location); err != nil {
		return config{}, fmt.Errorf("parsing config location: %w", err)
	}

	modelConfigFile, err := defaults.ModelConfigFile(location.Pool.ModelConfigFile, location.BasePath)
	if err != nil {
		return config{}, fmt.Errorf("resolving model config file: %w", err)
	}
	if err := models.UpgradeModelConfig(modelConfigFile); err != nil {
		return config{}, fmt.Errorf("upgrading model config file: %w", err)
	}

	data, err := os.ReadFile(modelConfigFile)
	if err != nil {
		return config{}, fmt.Errorf("reading config file: %w", err)
	}

	doc := struct {
		Version int    `yaml:"version"`
		KMS     config `yaml:"kms"`
	}{
		KMS: cfg,
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return config{}, fmt.Errorf("unmarshaling config file: %w", err)
	}

	switch doc.Version {
	case configVersion:
		cfg = doc.KMS
	default:
		return config{}, fmt.Errorf("config file: unsupported version %d", doc.Version)
	}

	cfg.Pool.ModelConfigFile = modelConfigFile

	help, err := conf.Parse(prefix, &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help)
		}
		return config{}, fmt.Errorf("parsing config: %w", err)
	}
	if err := os.Setenv("KRONK_HF_TOKEN", cfg.HfToken); err != nil {
		return config{}, fmt.Errorf("setting Hugging Face token: %w", err)
	}
	if err := os.Setenv("KRONK_LLAMA_LOG", strconv.Itoa(cfg.LlamaLog)); err != nil {
		return config{}, fmt.Errorf("setting llama log level: %w", err)
	}

	return cfg, nil
}

func resolveAuthorizationSettings(mode auth.Mode, legacyInference, legacyManagement, mcpAuthEnabled bool) (bool, bool, bool) {
	if mode.IsZero() {
		inferenceEnabled := legacyInference
		managementEnabled := legacyManagement || inferenceEnabled || mcpAuthEnabled
		return inferenceEnabled, managementEnabled, managementEnabled
	}

	switch mode {
	case auth.Open:
		return false, false, mcpAuthEnabled

	case auth.Management:
		return false, true, true

	case auth.Authenticated, auth.FullProtected:
		return true, true, true
	}

	return false, false, false
}

func validateTimeoutConfig(inferenceTimeout, writeTimeout time.Duration) error {
	if inferenceTimeout <= 0 {
		return errors.New("configuration: web inference timeout must be greater than zero")
	}
	if writeTimeout != 0 && writeTimeout <= inferenceTimeout {
		return errors.New("configuration: web write timeout must be disabled or greater than the inference timeout")
	}

	return nil
}

func validateAdminConfig(adminAuth, webAdmin bool, passwordSHA256, authHost string) error {
	if passwordSHA256 != "" {
		decoded, err := hex.DecodeString(passwordSHA256)
		if err != nil || len(decoded) != 32 {
			return errors.New("configuration: web admin password SHA-256 must be exactly 64 hexadecimal characters")
		}
	}
	if webAdmin && adminAuth && passwordSHA256 == "" {
		return errors.New("configuration: protected web admin requires a password SHA-256")
	}
	if webAdmin && adminAuth && passwordSHA256 != "" && authHost != "" {
		return errors.New("configuration: web password login does not support an external auth host")
	}

	return nil
}
