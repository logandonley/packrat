package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/logandonley/packrat/pkg/config"
	"github.com/logandonley/packrat/pkg/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// formatSize formats a file size in bytes to a human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// ListCmd returns the list command for displaying available backups
func ListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [service]",
		Short: "List available backups",
		Long: `List available backups for all services or a specific service.
If no service is specified, backups for all services will be listed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			var cfg config.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			if storage.Debug {
				fmt.Printf("Loaded configuration: %+v\n", cfg)
				fmt.Printf("Synology config: %+v\n", cfg.Backup.Synology)
			}

			// Create Synology storage
			synologyConfig := &storage.SynologyConfig{
				Host:     cfg.Backup.Synology.Host,
				Port:     cfg.Backup.Synology.Port,
				Username: cfg.Backup.Synology.Username,
				KeyFile:  cfg.Backup.Synology.KeyFile,
				Path:     cfg.Backup.Synology.Path,
			}

			if storage.Debug {
				fmt.Printf("Storage config: %+v\n", synologyConfig)
			}
			synology, err := storage.NewSynologyStorage(synologyConfig)
			if err != nil {
				return fmt.Errorf("failed to initialize Synology storage: %w", err)
			}
			defer synology.Close()

			// Get service filter
			var serviceFilter string
			if len(args) > 0 {
				serviceFilter = args[0]
			}

			// List backups for each service
			for serviceName, service := range cfg.Services {
				// Skip if filter is set and doesn't match
				if serviceFilter != "" && serviceFilter != serviceName {
					continue
				}

				fmt.Printf("\nBackups for service: %s\n", serviceName)
				fmt.Printf("Service path: %s\n", service.Path)
				if service.Docker != nil {
					fmt.Printf("Docker container: %s\n", service.Docker.Container)
				}

				// List backups from Synology
				backups, err := synology.List(serviceName)
				if err != nil {
					return fmt.Errorf("failed to list backups for %s: %w", serviceName, err)
				}

				if len(backups) == 0 {
					fmt.Println("No backups found")
					continue
				}

				// Sort backups by modification time (newest first)
				sort.Slice(backups, func(i, j int) bool {
					return backups[i].ModTime > backups[j].ModTime
				})

				fmt.Println("\nAvailable backups:")
				fmt.Printf("%-40s %-15s %s\n", "NAME", "SIZE", "MODIFIED")
				fmt.Println(strings.Repeat("-", 70))
				for _, backup := range backups {
					fmt.Printf("%-40s %-15s %s\n",
						backup.Name,
						formatSize(backup.Size),
						backup.ModTime,
					)
				}
			}

			return nil
		},
	}

	return cmd
}
