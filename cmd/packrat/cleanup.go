package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [service]",
	Short: "Remove old backups",
	Long: `Remove old backups while keeping the most recent ones according to retention settings.
If a service name is provided, only clean up that service's backups.
Otherwise, clean up backups for all services.

The number of backups to retain can be configured:
- Globally in the backup section: backup.retain_backups
- Per service: services.<n>.retain_backups

Service-specific settings override the global setting.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := createManager()
		if err != nil {
			return fmt.Errorf("failed to create backup manager: %w", err)
		}
		defer manager.Close()

		var serviceName string
		if len(args) > 0 {
			serviceName = args[0]
		}

		// Run cleanup
		deletedCounts, err := manager.CleanupBackups(serviceName)
		if err != nil {
			return fmt.Errorf("failed to clean up backups: %w", err)
		}

		totalDeleted := 0
		for service, count := range deletedCounts {
			totalDeleted += count
			if count > 0 {
				fmt.Printf("Deleted %d old backup(s) for service: %s\n", count, service)
			}
		}

		if serviceName != "" {
			if totalDeleted > 0 {
				fmt.Printf("Successfully cleaned up %d old backup(s) for service: %s\n", totalDeleted, serviceName)
			} else {
				fmt.Printf("No backups needed to be cleaned up for service: %s\n", serviceName)
			}
		} else {
			if totalDeleted > 0 {
				fmt.Printf("Successfully cleaned up %d old backup(s) across all services\n", totalDeleted)
			} else {
				fmt.Println("No backups needed to be cleaned up")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}
