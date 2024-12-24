package cmd

import (
	"fmt"

	"github.com/logandonley/packrat/pkg/backup"
	"github.com/logandonley/packrat/pkg/config"
	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// BackupCmd returns the backup command for creating backups of services
func BackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup [service]",
		Short: "Backup a specific service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := args[0]

			// Load configuration
			var cfg config.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Load encryption key
			key, _, err := crypto.LoadKey(cfg.Encryption.KeyFile)
			if err != nil {
				return fmt.Errorf("failed to load encryption key: %w", err)
			}

			// Create backup manager
			manager, err := backup.NewManager(&cfg, key)
			if err != nil {
				return fmt.Errorf("failed to create backup manager: %w", err)
			}

			// Create backup
			fmt.Printf("Creating backup of service: %s\n", serviceName)
			if err := manager.CreateBackup(serviceName); err != nil {
				return fmt.Errorf("failed to create backup: %w", err)
			}

			fmt.Printf("Backup of service %s completed successfully\n", serviceName)
			return nil
		},
	}

	return cmd
} 