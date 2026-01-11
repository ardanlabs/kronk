// Package cli provides documentation tooling for the CLI docs.
package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type command struct {
	Name        string
	Short       string
	Long        string
	Usage       string
	Subcommands []subcommand
}

type subcommand struct {
	Name        string
	Short       string
	Usage       string
	Flags       []flag
	EnvVars     []envVar
	Examples    []string
	Subcommands []subcommand
}

type flag struct {
	Name        string
	Description string
}

type envVar struct {
	Name        string
	Default     string
	Description string
}

var outputDir = "cmd/server/api/frontends/bui/src/components"

func Run() error {
	commands := []command{
		catalogCommand(),
		libsCommand(),
		modelCommand(),
		runCommand(),
		securityCommand(),
		serverCommand(),
	}

	for _, cmd := range commands {
		if err := generateCLIDoc(cmd); err != nil {
			return fmt.Errorf("generating %s docs: %w", cmd.Name, err)
		}
	}

	fmt.Println("CLI documentation generated successfully")

	return nil
}

func generateCLIDoc(cmd command) error {
	tsx := generateCLITSX(cmd)

	filename := fmt.Sprintf("DocsCLI%s.tsx", cases.Title(language.English).String(cmd.Name))
	outputPath := outputDir + "/" + filename

	if err := os.WriteFile(outputPath, []byte(tsx), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Generated %s\n", outputPath)

	return nil
}

func generateCLITSX(cmd command) string {
	var b strings.Builder

	componentName := fmt.Sprintf("DocsCLI%s", cases.Title(language.English).String(cmd.Name))

	b.WriteString(fmt.Sprintf("export default function %s() {\n", componentName))
	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")

	b.WriteString("      <div className=\"page-header\">\n")
	b.WriteString(fmt.Sprintf("        <h2>%s</h2>\n", cmd.Name))
	b.WriteString(fmt.Sprintf("        <p>%s</p>\n", escapeJSX(cmd.Short)))
	b.WriteString("      </div>\n\n")

	b.WriteString("      <div className=\"doc-layout\">\n")
	b.WriteString("        <div className=\"doc-content\">\n")

	b.WriteString("          <div className=\"card\" id=\"usage\">\n")
	b.WriteString("            <h3>Usage</h3>\n")
	b.WriteString("            <pre className=\"code-block\">\n")
	b.WriteString(fmt.Sprintf("              <code>%s</code>\n", escapeJSX(cmd.Usage)))
	b.WriteString("            </pre>\n")
	b.WriteString("          </div>\n")

	if len(cmd.Subcommands) > 0 {
		b.WriteString("\n          <div className=\"card\" id=\"subcommands\">\n")
		b.WriteString("            <h3>Subcommands</h3>\n")

		for _, sub := range cmd.Subcommands {
			writeSubcommand(&b, sub, "cmd")
		}

		b.WriteString("          </div>\n")
	}

	b.WriteString("        </div>\n")

	b.WriteString("\n        <nav className=\"doc-sidebar\">\n")
	b.WriteString("          <div className=\"doc-sidebar-content\">\n")
	b.WriteString("            <div className=\"doc-index-section\">\n")
	b.WriteString("              <a href=\"#usage\" className=\"doc-index-header\">Usage</a>\n")
	b.WriteString("            </div>\n")

	if len(cmd.Subcommands) > 0 {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString("              <a href=\"#subcommands\" className=\"doc-index-header\">Subcommands</a>\n")
		b.WriteString("              <ul>\n")

		for _, sub := range cmd.Subcommands {
			writeSubcommandNav(&b, sub, "cmd")
		}

		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	b.WriteString("          </div>\n")
	b.WriteString("        </nav>\n")

	b.WriteString("      </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

func writeSubcommand(b *strings.Builder, sub subcommand, prefix string) {
	anchor := toAnchor(prefix + "-" + sub.Name)

	fmt.Fprintf(b, "\n            <div className=\"doc-section\" id=\"%s\">\n", anchor)
	fmt.Fprintf(b, "              <h4>%s</h4>\n", sub.Name)
	fmt.Fprintf(b, "              <p className=\"doc-description\">%s</p>\n", escapeJSX(sub.Short))
	b.WriteString("              <pre className=\"code-block\">\n")
	fmt.Fprintf(b, "                <code>%s</code>\n", escapeJSX(sub.Usage))
	b.WriteString("              </pre>\n")

	if len(sub.Flags) > 0 {
		b.WriteString("              <table className=\"flags-table\">\n")
		b.WriteString("                <thead>\n")
		b.WriteString("                  <tr>\n")
		b.WriteString("                    <th>Flag</th>\n")
		b.WriteString("                    <th>Description</th>\n")
		b.WriteString("                  </tr>\n")
		b.WriteString("                </thead>\n")
		b.WriteString("                <tbody>\n")

		for _, f := range sub.Flags {
			b.WriteString("                  <tr>\n")
			fmt.Fprintf(b, "                    <td><code>%s</code></td>\n", escapeJSX(f.Name))
			fmt.Fprintf(b, "                    <td>%s</td>\n", escapeJSX(f.Description))
			b.WriteString("                  </tr>\n")
		}

		b.WriteString("                </tbody>\n")
		b.WriteString("              </table>\n")
	}

	if len(sub.EnvVars) > 0 {
		b.WriteString("              <h5>Environment Variables</h5>\n")
		b.WriteString("              <table className=\"flags-table\">\n")
		b.WriteString("                <thead>\n")
		b.WriteString("                  <tr>\n")
		b.WriteString("                    <th>Variable</th>\n")
		b.WriteString("                    <th>Default</th>\n")
		b.WriteString("                    <th>Description</th>\n")
		b.WriteString("                  </tr>\n")
		b.WriteString("                </thead>\n")
		b.WriteString("                <tbody>\n")

		for _, e := range sub.EnvVars {
			b.WriteString("                  <tr>\n")
			fmt.Fprintf(b, "                    <td><code>%s</code></td>\n", escapeJSX(e.Name))
			fmt.Fprintf(b, "                    <td>%s</td>\n", escapeJSX(e.Default))
			fmt.Fprintf(b, "                    <td>%s</td>\n", escapeJSX(e.Description))
			b.WriteString("                  </tr>\n")
		}

		b.WriteString("                </tbody>\n")
		b.WriteString("              </table>\n")
	}

	if len(sub.Examples) > 0 {
		b.WriteString("              <h5>Example</h5>\n")
		b.WriteString("              <pre className=\"code-block\">\n")
		fmt.Fprintf(b, "                <code>{`%s`}</code>\n", escapeTemplateLiteral(strings.Join(sub.Examples, "\n\n")))
		b.WriteString("              </pre>\n")
	}

	for _, nested := range sub.Subcommands {
		writeSubcommand(b, nested, prefix+"-"+sub.Name)
	}

	b.WriteString("            </div>\n")
}

func writeSubcommandNav(b *strings.Builder, sub subcommand, prefix string) {
	anchor := toAnchor(prefix + "-" + sub.Name)
	fmt.Fprintf(b, "                <li><a href=\"#%s\">%s</a></li>\n", anchor, sub.Name)

	for _, nested := range sub.Subcommands {
		writeSubcommandNav(b, nested, prefix+"-"+sub.Name)
	}
}

func toAnchor(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, " ", "-")

	return s
}

func escapeJSX(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")

	return s
}

func escapeTemplateLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")

	return s
}

// =============================================================================

func catalogCommand() command {
	return command{
		Name:  "catalog",
		Short: "Manage model catalog - list and update available models.",
		Long:  "Manage model catalog - list and update available models",
		Usage: "kronk catalog <command> [flags]",
		Subcommands: []subcommand{
			{
				Name:  "list",
				Short: "List catalog models.",
				Usage: "kronk catalog list [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
					{Name: "--filter-category <string>", Description: "Filter catalogs by category name (substring match)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
				},
				Examples: []string{
					"# List all catalog models\nkronk catalog list",
					"# List models with local mode (no server required)\nkronk catalog list --local",
					"# Filter models by category\nkronk catalog list --filter-category embedding",
				},
			},
			{
				Name:  "pull",
				Short: "Pull a model from the catalog.",
				Usage: "kronk catalog pull <MODEL_ID> [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
				},
				Examples: []string{
					"# Pull a model from the catalog\nkronk catalog pull llama-3.2-1b-q4",
					"# Pull with local mode\nkronk catalog pull llama-3.2-1b-q4 --local",
				},
			},
			{
				Name:  "show",
				Short: "Show catalog model information.",
				Usage: "kronk catalog show <MODEL_ID> [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
				},
				Examples: []string{
					"# Show details for a specific model\nkronk catalog show llama-3.2-1b-q4",
					"# Show with local mode\nkronk catalog show llama-3.2-1b-q4 --local",
				},
			},
			{
				Name:  "update",
				Short: "Update the model catalog.",
				Usage: "kronk catalog update [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
				},
				Examples: []string{
					"# Update the catalog from remote source\nkronk catalog update",
					"# Update with local mode\nkronk catalog update --local",
				},
			},
		},
	}
}

func libsCommand() command {
	return command{
		Name:  "libs",
		Short: "Install or upgrade llama.cpp libraries.",
		Long:  "Install or upgrade llama.cpp libraries",
		Usage: "kronk libs [flags]",
		Subcommands: []subcommand{
			{
				Name:  "(default)",
				Short: "Install or upgrade llama.cpp libraries.",
				Usage: "kronk libs [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
					{Name: "KRONK_ARCH", Default: "runtime.GOARCH", Description: "The architecture to install (local mode)"},
					{Name: "KRONK_LIB_PATH", Default: "$HOME/kronk/libraries", Description: "The path to the libraries directory (local mode)"},
					{Name: "KRONK_OS", Default: "runtime.GOOS", Description: "The operating system to install (local mode)"},
					{Name: "KRONK_PROCESSOR", Default: "cpu", Description: "Options: cpu, cuda, metal, vulkan (local mode)"},
				},
				Examples: []string{
					"# Install libraries using the server\nkronk libs",
					"# Install libraries locally\nkronk libs --local",
					"# Install with Metal support on macOS\nKRONK_PROCESSOR=metal kronk libs --local",
				},
			},
		},
	}
}

func modelCommand() command {
	return command{
		Name:  "model",
		Short: "Manage models - list, pull, remove, show, and check running models.",
		Long:  "Manage models - list, pull, remove, show, and check running models",
		Usage: "kronk model <command> [flags]",
		Subcommands: []subcommand{
			{
				Name:  "index",
				Short: "Rebuild the model index for fast model access.",
				Usage: "kronk model index [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
					{Name: "KRONK_MODELS", Default: "$HOME/kronk/models", Description: "The path to the models directory (local mode)"},
				},
				Examples: []string{
					"# Rebuild the model index\nkronk model index",
					"# Rebuild with local mode\nkronk model index --local",
				},
			},
			{
				Name:  "list",
				Short: "List models.",
				Usage: "kronk model list [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
					{Name: "KRONK_MODELS", Default: "$HOME/kronk/models", Description: "The path to the models directory (local mode)"},
				},
				Examples: []string{
					"# List all models\nkronk model list",
					"# List with local mode\nkronk model list --local",
				},
			},
			{
				Name:  "ps",
				Short: "List running models.",
				Usage: "kronk model ps",
				Flags: []flag{},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server"},
				},
				Examples: []string{
					"# List running models\nkronk model ps",
				},
			},
			{
				Name:  "pull",
				Short: "Pull a model from the web.",
				Usage: "kronk model pull <MODEL_URL> [MMPROJ_URL] [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
					{Name: "KRONK_MODELS", Default: "$HOME/kronk/models", Description: "The path to the models directory (local mode)"},
				},
				Examples: []string{
					"# Pull a model from a URL\nkronk model pull https://huggingface.co/.../model.gguf",
					"# Pull with local mode\nkronk model pull https://huggingface.co/.../model.gguf --local",
					"# Pull a vision model with mmproj file\nkronk model pull <MODEL_URL> <MMPROJ_URL>",
				},
			},
			{
				Name:  "remove",
				Short: "Remove a model.",
				Usage: "kronk model remove <MODEL_NAME> [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
					{Name: "KRONK_MODELS", Default: "$HOME/kronk/models", Description: "The path to the models directory (local mode)"},
				},
				Examples: []string{
					"# Remove a model\nkronk model remove llama-3.2-1b-q4",
					"# Remove with local mode\nkronk model remove llama-3.2-1b-q4 --local",
				},
			},
			{
				Name:  "show",
				Short: "Show information for a model.",
				Usage: "kronk model show <MODEL_NAME> [flags]",
				Flags: []flag{
					{Name: "--local", Description: "Run without the model server"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_TOKEN", Default: "", Description: "Authentication token for the kronk server (required when auth enabled)"},
					{Name: "KRONK_WEB_API_HOST", Default: "localhost:8080", Description: "IP Address for the kronk server (web mode)"},
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories (local mode)"},
					{Name: "KRONK_MODELS", Default: "$HOME/kronk/models", Description: "The path to the models directory (local mode)"},
				},
				Examples: []string{
					"# Show model information\nkronk model show llama-3.2-1b-q4",
					"# Show with local mode\nkronk model show llama-3.2-1b-q4 --local",
				},
			},
		},
	}
}

func runCommand() command {
	return command{
		Name:  "run",
		Short: "Run an interactive chat session with a model.",
		Long:  "Run an interactive chat session with a local model (REPL mode)",
		Usage: "kronk run <MODEL_NAME> [flags]",
		Subcommands: []subcommand{
			{
				Name:  "flags",
				Short: "Available flags for the run command.",
				Usage: "kronk run <MODEL_NAME> [flags]",
				Flags: []flag{
					{Name: "--instances <int>", Description: "Number of model instances to load (default: 1)"},
					{Name: "--max-tokens <int>", Description: "Maximum tokens for response (default: 2048)"},
					{Name: "--temperature <float>", Description: "Temperature for sampling (default: 0.7)"},
					{Name: "--top-p <float>", Description: "Top-p for sampling (default: 0.9)"},
					{Name: "--top-k <int>", Description: "Top-k for sampling (default: 40)"},
					{Name: "--base-path <string>", Description: "Base path for kronk data (models, catalogs, templates)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories"},
					{Name: "KRONK_MODELS", Default: "$HOME/kronk/models", Description: "The path to the models directory"},
				},
				Examples: []string{
					"# Start an interactive chat with a model\nkronk run Qwen3-8B-Q8_0",
					"# Run with custom sampling parameters\nkronk run Qwen3-8B-Q8_0 --temperature 0.5 --top-p 0.95",
					"# Run with higher token limit\nkronk run Qwen3-8B-Q8_0 --max-tokens 4096",
				},
			},
		},
	}
}

func securityCommand() command {
	return command{
		Name:  "security",
		Short: "Manage security - tokens and access control.",
		Long:  "Manage security - tokens and access control",
		Usage: "kronk security <command> [flags]",
		Subcommands: []subcommand{
			{
				Name:  "key",
				Short: "Manage private keys - create and delete private keys.",
				Usage: "kronk security key <command> [flags]",
				Subcommands: []subcommand{
					{
						Name:  "create",
						Short: "Create a new private key and add it to the keystore.",
						Usage: "kronk security key create [flags]",
						Flags: []flag{
							{Name: "--local", Description: "Run without the model server"},
						},
						EnvVars: []envVar{
							{Name: "KRONK_TOKEN", Default: "", Description: "Admin token (required when auth enabled)"},
						},
						Examples: []string{
							"# Create a new private key\nexport KRONK_TOKEN=<admin-token>\nkronk security key create",
						},
					},
					{
						Name:  "delete",
						Short: "Delete a private key by its key ID.",
						Usage: "kronk security key delete --keyid <KEY_ID> [flags]",
						Flags: []flag{
							{Name: "--keyid <string>", Description: "The key ID to delete (required)"},
							{Name: "--local", Description: "Run without the model server"},
						},
						EnvVars: []envVar{
							{Name: "KRONK_TOKEN", Default: "", Description: "Admin token (required when auth enabled)"},
						},
						Examples: []string{
							"# Delete a private key\nexport KRONK_TOKEN=<admin-token>\nkronk security key delete --keyid abc123",
						},
					},
					{
						Name:  "list",
						Short: "List all private keys in the system.",
						Usage: "kronk security key list [flags]",
						Flags: []flag{
							{Name: "--local", Description: "Run without the model server"},
						},
						EnvVars: []envVar{
							{Name: "KRONK_TOKEN", Default: "", Description: "Admin token (required when auth enabled)"},
						},
						Examples: []string{
							"# List all private keys\nexport KRONK_TOKEN=<admin-token>\nkronk security key list",
						},
					},
				},
			},
			{
				Name:  "token",
				Short: "Manage tokens - create and manage security tokens.",
				Usage: "kronk security token <command> [flags]",
				Subcommands: []subcommand{
					{
						Name:  "create",
						Short: "Create a security token.",
						Usage: "kronk security token create [flags]",
						Flags: []flag{
							{Name: "--local", Description: "Run without the model server"},
							{Name: "--duration <duration>", Description: "Token duration (e.g., 1h, 24h, 720h)"},
							{Name: "--endpoints <list>", Description: "Endpoints with optional rate limits"},
						},
						EnvVars: []envVar{
							{Name: "KRONK_TOKEN", Default: "", Description: "Admin token (required when auth enabled)"},
						},
						Examples: []string{
							"# Create a token with 24 hour duration\nexport KRONK_TOKEN=<admin-token>\nkronk security token create --duration 24h --endpoints chat-completions,embeddings",
							"# Create a token with rate limits\nkronk security token create --duration 720h --endpoints \"chat-completions:1000/day,embeddings:unlimited\"",
						},
					},
				},
			},
		},
	}
}

func serverCommand() command {
	return command{
		Name:  "server",
		Short: "Manage model server - start, stop, logs.",
		Long:  "Manage model server - start, stop, logs",
		Usage: "kronk server <command> [flags]",
		Subcommands: []subcommand{
			{
				Name:  "start",
				Short: "Start Kronk model server.",
				Usage: "kronk server start [flags]",
				Flags: []flag{
					{Name: "-d, --detach", Description: "Run server in the background"},
					{Name: "--api-host <string>", Description: "API host address (e.g., localhost:8080)"},
					{Name: "--debug-host <string>", Description: "Debug host address (e.g., localhost:8090)"},
					{Name: "--auth-enabled", Description: "Enable local authentication"},
					{Name: "--device <string>", Description: "Device to use for inference (e.g., cuda, metal)"},
					{Name: "--max-instances <int>", Description: "Maximum model instances"},
					{Name: "--models-in-cache <int>", Description: "Maximum models in cache"},
					{Name: "--cache-ttl <duration>", Description: "Cache TTL duration (e.g., 5m, 1h)"},
					{Name: "--model-config-file <string>", Description: "Special config file for model specific config"},
					{Name: "--llama-log <int>", Description: "Llama log level (0=off, 1=on)"},
				},
				EnvVars: []envVar{
					{Name: "KRONK_BASE_PATH", Default: "$HOME/kronk", Description: "Base path for kronk data directories"},
				},
				Examples: []string{
					"# Start the server in foreground\nkronk server start",
					"# Start the server in background\nkronk server start -d",
					"# View all server environment settings\nkronk server start --help",
				},
			},
			{
				Name:  "stop",
				Short: "Stop the Kronk model server by sending SIGTERM.",
				Usage: "kronk server stop",
				Flags: []flag{},
				Examples: []string{
					"# Stop the server\nkronk server stop",
				},
			},
			{
				Name:  "logs",
				Short: "Stream the Kronk model server logs (tail -f).",
				Usage: "kronk server logs",
				Flags: []flag{},
				Examples: []string{
					"# Stream server logs\nkronk server logs",
				},
			},
		},
	}
}
