package config

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "config",
	Short: "Create or edit model and server settings in model_config.yaml (interactive)",
	Long: `Create or edit model and server settings in model_config.yaml (interactive)

Pick the config file, then configure a model or the server settings (kms:).

For a model, say whether the config is for this machine or another one. This
machine's hardware is detected; for another machine, enter its GPUs, GPU
memory and RAM. Search and pick an installed or catalog model (chat, audio or
image), pick the entry or a new variant, see what AutoTune picks for it on that
hardware, answer the questions, review the entry with a memory check and save
it. Only the settings the model can use are asked: a model that doesn't
generate text (embeddings, rerank, decision) gets no sampling, thinking or chat
template questions. A catalog model needs no download to be configured; after
saving, the tool offers to download it, or to copy its pull command.

For the server, pick the sections to change and answer their questions. The
questions come from the server's own settings.

Only answered settings are written; everything else in the file, including
comments, is kept. The previous file is kept as .bak. Esc goes back.

The command works on local files and needs no server. The file the server
reads is KRONK_POOL_MODEL_CONFIG_FILE when set, otherwise
<base-path>/models/model_config.yaml. The tool also lists model_config*.yaml
files in the current directory, and can save to any path.

Environment Variables:
      KRONK_BASE_PATH               Base path for kronk data (models, libraries, catalog, model_config)
      KRONK_POOL_MODEL_CONFIG_FILE  Model config file the server reads`,
	Args: cobra.NoArgs,
	RunE: main,
}

func init() {
	Cmd.Flags().String("theme", "auto", "Colors: auto (from the terminal's background), light or dark")
}

func main(cmd *cobra.Command, args []string) error {
	return run(cmd)
}
