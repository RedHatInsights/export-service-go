package main

import (
	"os"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func createRootCommand(cfg *config.ExportConfig, log *zap.SugaredLogger) *cobra.Command {

	// rootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use: "export-service",
	}

	var expiredExportCleanerCmd = &cobra.Command{
		Use:   "expired_export_cleaner",
		Short: "Run the expired export cleaner",
		Run: func(cmd *cobra.Command, args []string) {
			startExpiredExportCleaner(cfg, log)
		},
	}

	rootCmd.AddCommand(expiredExportCleanerCmd)

	return rootCmd
}

func main() {
	cfg := config.ExportCfg
	log := logger.Log

	cmd := createRootCommand(cfg, log)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
