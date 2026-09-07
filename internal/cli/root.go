package cli

import (
	"github.com/spf13/cobra"
)

var (
	configPath string
	dataDir    string
)

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:          "henia",
	Short:        "Build, lint and deploy portable AI skills",
	Long:         "Henia compiles canonical Markdown skills, lints their quality, and deploys to multiple harnesses.",
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(newBuildCommand(), newLintCommand())
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "henia.toml", "Config file path")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", ".henia/sources", "Data directory for sources")
}
