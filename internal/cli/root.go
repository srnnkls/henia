package cli

import (
	"github.com/spf13/cobra"
)

var (
	configPath string
)

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:          "henia",
	Short:        "Build and lint portable AI skills",
	Long:         "Henia compiles canonical Markdown skills into build artifacts and lints their quality.",
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(newBuildCommand(), newLintCommand())
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "henia.toml", "Config file path")
}
