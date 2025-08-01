package main

import (
	"fmt"
	"os"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/logger"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func createRootCommand(cfg *config.ExportConfig, log *zap.SugaredLogger) *cobra.Command {
	// rootCmd represents the base command when called without any subcommands
	rootCmd := &cobra.Command{
		Use: "export-service",
	}

	expiredExportCleanerCmd := &cobra.Command{
		Use:   "expired_export_cleaner",
		Short: "Run the expired export cleaner",
		Run: func(cmd *cobra.Command, args []string) {
			startExpiredExportCleaner(cfg, log)
		},
	}

	rootCmd.AddCommand(expiredExportCleanerCmd)

	apiServerCmd := &cobra.Command{
		Use:   "api_server",
		Short: "Run the api server",
		Run: func(cmd *cobra.Command, args []string) {
			startApiServer(cfg, log)
		},
	}

	rootCmd.AddCommand(apiServerCmd)

	migrateDbCmd := &cobra.Command{
		Use:   "migrate_db",
		Short: "Run the db migration",
	}

	// upCmd represents the up command
	upCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade to a later version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("up called")
			return performDbMigration(cfg, log, "up")
		},
	}

	// downCmd represents the down command
	downCmd := &cobra.Command{
		Use:   "downgrade",
		Short: "Revert to a previous version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("down called")
			return performDbMigration(cfg, log, "down")
		},
	}

	rootCmd.AddCommand(migrateDbCmd)

	migrateDbCmd.AddCommand(upCmd)
	migrateDbCmd.AddCommand(downCmd)

	return rootCmd
}

func main() {
	cfg := config.Get()
	log := logger.Get()

	cmd := createRootCommand(cfg, log)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
