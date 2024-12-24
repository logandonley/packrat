package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/logandonley/packrat/pkg/backup"
	"github.com/logandonley/packrat/pkg/cmd"
	"github.com/logandonley/packrat/pkg/config"
	"github.com/logandonley/packrat/pkg/crypto"
	"github.com/logandonley/packrat/pkg/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	debug   bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "packrat",
	Short: "A secure backup tool",
	Long: `Packrat is a secure backup tool that encrypts and stores your data
in various storage backends like Synology NAS.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		storage.Debug = debug
		if debug {
			log.Println("Debug mode enabled")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/packrat/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")

	rootCmd.AddCommand(cmd.BackupCmd())
	rootCmd.AddCommand(cmd.InitCmd())
	rootCmd.AddCommand(cmd.RekeyCmd())
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in ~/.config/packrat directory
		configDir := filepath.Join(home, ".config", "packrat")
		viper.AddConfigPath(configDir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Read in environment variables that match
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		if debug {
			fmt.Println("Using config file:", viper.ConfigFileUsed())
			fmt.Printf("Config contents: %+v\n", viper.AllSettings())
		}
	} else {
		fmt.Printf("Error reading config file: %v\n", err)
	}
}

// createManager creates a new backup manager with the current configuration
func createManager() (*backup.Manager, error) {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if debug {
		fmt.Printf("Config after unmarshal: %+v\n", cfg)
		fmt.Printf("Key file path: %q\n", cfg.Encryption.KeyFile)
		fmt.Printf("All viper settings: %+v\n", viper.AllSettings())
	}

	// Load the encryption key
	key, _, err := crypto.LoadKey(cfg.Encryption.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load encryption key: %w", err)
	}

	manager, err := backup.NewManager(&cfg, key)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup manager: %w", err)
	}

	return manager, nil
}
