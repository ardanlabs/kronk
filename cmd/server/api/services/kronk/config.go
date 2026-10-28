package kronk

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"go.yaml.in/yaml/v2"
)

var supportedConfigVersions = []int{1, 2}

type config struct {
	conf.Version `yaml:"-"`
	Web          struct {
		ReadTimeout        time.Duration `conf:"help:HTTP read timeout" yaml:"read-timeout"`
		WriteTimeout       time.Duration `conf:"help:HTTP write timeout. 0 disables it; otherwise it must be longer than the inference timeout" yaml:"write-timeout"`
		InferenceTimeout   time.Duration `conf:"help:Total time one inference request may take" yaml:"inference-timeout"`
		IdleTimeout        time.Duration `conf:"help:HTTP idle timeout" yaml:"idle-timeout"`
		ShutdownTimeout    time.Duration `conf:"help:Time to finish requests when the server stops" yaml:"shutdown-timeout"`
		APIHost            string        `conf:"help:Address the API listens on (host:port). 0.0.0.0 accepts connections from other machines" yaml:"api-host"`
		DebugHost          string        `conf:"help:Address of the debug server: metrics and pprof (host:port)" yaml:"debug-host"`
		CORSAllowedOrigins []string      `conf:"help:Origins allowed to call the API from a browser" yaml:"cors-allowed-origins"`
		Admin              struct {
			Enabled        bool   `conf:"help:Serve the browser admin UI at /admin/" yaml:"enabled"`
			PasswordSHA256 string `conf:"mask,help:SHA-256 digest of the browser admin login password" yaml:"password-sha-256"`
		} `yaml:"admin"`
	} `yaml:"web"`
	Auth struct {
		Host string `conf:"help:External auth service host. Empty uses the local auth" yaml:"host"`
		TLS  struct {
			Enabled    bool   `conf:"help:Use TLS for the external auth service" yaml:"enabled"`
			CAFile     string `conf:"help:CA certificate file for the external auth service" yaml:"ca-file"`
			ServerName string `conf:"help:TLS server name for the external auth service" yaml:"server-name"`
		} `yaml:"tls"`
		Local struct {
			Issuer string `conf:"help:Issuer name in the tokens the local auth creates" yaml:"issuer"`
		} `yaml:"local"`
	} `yaml:"auth"`
	Authorization struct {
		Mode auth.Mode `conf:"help:Which endpoints need a token: open / management / authenticated / full-protected" yaml:"mode"`
	} `yaml:"authorization"`
	MCP struct {
		Enabled     bool   `conf:"help:Run the embedded MCP service" yaml:"enabled"`
		Host        string `conf:"help:External MCP host. Setting it disables the embedded service" yaml:"host"`
		AuthEnabled bool   `conf:"help:Require a Kronk admin token for MCP access" yaml:"auth-enabled"`
		BraveAPIKey string `conf:"mask,help:Brave Search API key for the embedded MCP service" yaml:"brave-api-key"`
	} `yaml:"mcp"`
	Download struct {
		Enabled bool `conf:"help:Allow model downloads through the server" yaml:"enabled"`
	} `yaml:"download"`
	Tempo struct {
		Host        string  `conf:"help:Tempo tracing host (host:port)" yaml:"host"`
		ServiceName string  `conf:"help:Service name in the traces" yaml:"service-name"`
		Probability float64 `conf:"help:Share of requests traced (0 to 1)" yaml:"probability"`
	} `yaml:"tempo"`
	Pool struct {
		ModelConfigFile string        `yaml:"-"`
		BudgetPercent   int           `conf:"help:Percent (1 to 100) of system and GPU memory the loaded models may use" yaml:"budget-percent"`
		ModelsInPool    int           `conf:"help:Most models kept loaded at once regardless of memory" yaml:"models-in-pool"`
		TTL             time.Duration `conf:"help:Unload a model after it is idle this long. 0 keeps it loaded" yaml:"ttl"`
	} `yaml:"pool"`
	BasePath             string `conf:"help:Base path for Kronk data: models and libraries" yaml:"base-path"`
	LibPath              string `conf:"help:Path to the llama.cpp libraries" yaml:"lib-path"`
	BuckyLibPath         string `conf:"help:Path to the whisper.cpp libraries" yaml:"bucky-lib-path"`
	MalinaLibPath        string `conf:"help:Path to the stable-diffusion.cpp libraries" yaml:"malina-lib-path"`
	LibVersion           string `conf:"help:llama.cpp library version. Optionally VERSION@sha256:DIGEST" yaml:"lib-version"`
	LibDownloadEnabled   bool   `conf:"help:Download the llama.cpp libraries at startup when missing" yaml:"lib-download-enabled"`
	MediaBackendsEnabled bool   `conf:"help:Download and load the audio (Bucky) and image (Malina) backends at startup" yaml:"media-backends-enabled"`
	LibVerifyEnabled     bool   `conf:"help:Verify the llama.cpp and whisper.cpp libraries before loading them" yaml:"lib-verify-enabled"`
	Arch                 string `conf:"help:Architecture override: amd64 / arm64" yaml:"arch"`
	OS                   string `conf:"help:Operating system override" yaml:"os"`
	Processor            string `conf:"help:Processor override: cpu / cuda / metal / rocm / vulkan" yaml:"processor"`
	AllowUpgrade         bool   `conf:"help:Allow automatic library upgrades" yaml:"allow-upgrade"`
	InsecureLogging      bool   `conf:"help:Log sensitive data such as messages and model config" yaml:"insecure-logging"`
	HfToken              string `conf:"mask,help:Hugging Face token for gated models" yaml:"hf-token"`
	LlamaLog             int    `conf:"help:llama.cpp log level: 0 off / 1 on" yaml:"llama-log"`
}

func newConfig() config {
	cfg := config{
		Build: kronk.Version,
		Desc:  "Kronk",
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
	cfg.Authorization.Mode = auth.Open
	cfg.MCP.Enabled = true
	cfg.Tempo.Host = "localhost:4317"
	cfg.Tempo.ServiceName = "kronk"
	cfg.Tempo.Probability = 0.25
	cfg.Pool.BudgetPercent = 90
	cfg.Pool.ModelsInPool = 10
	cfg.LibDownloadEnabled = true
	cfg.MediaBackendsEnabled = true
	cfg.LibVerifyEnabled = true

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

	if !slices.Contains(supportedConfigVersions, doc.Version) {
		return config{}, fmt.Errorf("config file: unsupported version %d", doc.Version)
	}
	cfg = doc.KMS

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

func resolveAuthorizationSettings(mode auth.Mode, mcpAuthEnabled bool) (bool, bool, bool) {
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
