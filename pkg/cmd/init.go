package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// InitCmd returns the init command
func InitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Packrat and set up encryption",
		Long: `Initialize Packrat by creating the necessary directories and setting up encryption.
This command will prompt for a password to generate the encryption key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get home directory
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			// Create config directory
			configDir := filepath.Join(home, ".config", "packrat")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			// Get password from user
			fmt.Print("Enter password for encryption: ")
			password, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			fmt.Println()

			// Generate and save key
			keyPath := filepath.Join(configDir, "key")
			if err := crypto.GenerateAndSaveKey(password, keyPath); err != nil {
				return fmt.Errorf("failed to generate and save key: %w", err)
			}

			// Create default config file
			configPath := filepath.Join(configDir, "config.yaml")
			defaultConfig := fmt.Sprintf(`encryption:
  key_file: %s

services:
  # Add your services here
  # example:
  #   path: /path/to/service
  #   schedule: "0 2 * * *"  # 2 AM daily
  #   docker:
  #     container: container_name
  #   exclude:
  #     - "**/tmp/**"
  #     - "**/.git"
  #     - "**/node_modules/**"
  #   retain_backups: 14  # Keep last 14 backups

backup:
  retain_backups: 7  # Global default: keep last 7 backups
  synology:
    host: nas.example.com
    port: 22
    username: user
    key_file: ~/.ssh/id_rsa
    path: ./backups/packrat/
  s3:
    # For Backblaze B2, use endpoint: https://s3.REGION.backblazeb2.com
    # For MinIO, use your MinIO server endpoint
    # For AWS S3, leave endpoint empty
    endpoint: ""
    region: us-east-1
    bucket: your-bucket-name
    access_key_id: your-access-key
    secret_access_key: your-secret-key
    path: backups/packrat/
`, keyPath)

			if err := os.WriteFile(configPath, []byte(defaultConfig), 0600); err != nil {
				return fmt.Errorf("failed to create config file: %w", err)
			}

			fmt.Printf("Initialization complete.\nKey saved to: %s\nConfig file created at: %s\n", keyPath, configPath)
			return nil
		},
	}

	return cmd
}
