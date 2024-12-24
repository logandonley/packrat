package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// RekeyCmd returns the rekey command
func RekeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rekey",
		Short: "Regenerate encryption key from password",
		Long: `Regenerate the encryption key from the original password.
This is useful if you've lost your key file but remember the password.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get home directory
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			// Get password from user
			fmt.Print("Enter original password: ")
			password, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			fmt.Println()

			// Generate and save key
			keyPath := filepath.Join(home, ".config", "packrat", "key")
			if err := crypto.GenerateAndSaveKey(password, keyPath); err != nil {
				return fmt.Errorf("failed to generate and save key: %w", err)
			}

			fmt.Printf("Key regenerated and saved to %s\n", keyPath)
			return nil
		},
	}

	return cmd
}
