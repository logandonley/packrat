package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/logandonley/packrat/pkg/backup"
	"github.com/logandonley/packrat/pkg/daemon"
	"github.com/spf13/cobra"
)

var (
	testMode bool
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the Packrat daemon for scheduled backups",
	Long: `Run the Packrat daemon which handles scheduled backups according to the configuration.
The daemon will run in the foreground and can be stopped with Ctrl+C.

In test mode (--test), it will validate:
- Configuration file syntax and permissions
- Service directories existence and permissions
- Docker connectivity (if configured)
- Synology connectivity
- Backup directory permissions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := createManager()
		if err != nil {
			return fmt.Errorf("failed to create backup manager: %w", err)
		}
		defer manager.Close()

		if testMode {
			return validateConfiguration(manager)
		}

		// Create and start the daemon
		d := daemon.New(manager.GetConfig(), manager)

		// Handle shutdown signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			d.Stop()
		}()

		// Run the daemon
		if err := d.Run(); err != nil {
			return fmt.Errorf("daemon error: %w", err)
		}

		return nil
	},
}

func validateConfiguration(manager *backup.Manager) error {
	cfg := manager.GetConfig()
	fmt.Println("🔍 Validating Packrat configuration...")

	// Check config directory
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "packrat")
	if err := validateDirectory(configDir, false); err != nil {
		return fmt.Errorf("config directory validation failed: %w", err)
	}
	fmt.Printf("✅ Config directory %s is accessible\n", configDir)

	// Check config file
	configFile := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configFile); err != nil {
		return fmt.Errorf("config file validation failed: %w", err)
	}
	fmt.Printf("✅ Config file %s is accessible\n", configFile)

	// Check each service
	for name, service := range cfg.Services {
		fmt.Printf("\n📁 Validating service: %s\n", name)

		// Validate service path
		if err := validateDirectory(service.Path, false); err != nil {
			return fmt.Errorf("service %s path validation failed: %w", name, err)
		}
		fmt.Printf("✅ Service path %s is accessible\n", service.Path)

		// Validate schedule if specified
		if service.Schedule != "" {
			if err := validateCronSchedule(service.Schedule); err != nil {
				return fmt.Errorf("service %s schedule validation failed: %w", name, err)
			}
			fmt.Printf("✅ Schedule %s is valid\n", service.Schedule)
		}

		// Validate Docker container if specified
		if service.Docker != nil {
			if err := validateDockerContainer(manager, service.Docker.Container); err != nil {
				return fmt.Errorf("service %s Docker validation failed: %w", name, err)
			}
			fmt.Printf("✅ Docker container %s is accessible\n", service.Docker.Container)
		}
	}

	// Test Synology connectivity
	fmt.Println("\n🔌 Testing Synology connectivity...")
	if err := validateSynologyConnection(manager); err != nil {
		return fmt.Errorf("synology connection validation failed: %w", err)
	}
	fmt.Println("✅ Successfully connected to Synology")

	fmt.Println("\n✨ All validation checks passed successfully!")
	return nil
}

func validateDirectory(path string, requireWritable bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", path)
		}
		return fmt.Errorf("cannot access directory: %s: %w", path, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Check if we can read the directory
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open directory: %s: %w", path, err)
	}
	f.Close()

	if requireWritable {
		// Try to create and remove a test file
		testFile := filepath.Join(path, ".packrat-test")
		if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
			return fmt.Errorf("directory is not writable: %s: %w", path, err)
		}
		os.Remove(testFile)
	}

	return nil
}

func validateCronSchedule(schedule string) error {
	err := daemon.ParseCronSchedule(schedule)
	return err
}

func validateDockerContainer(manager *backup.Manager, containerName string) error {
	return manager.ValidateDockerContainer(containerName)
}

func validateSynologyConnection(manager *backup.Manager) error {
	return manager.ValidateSynologyConnection()
}

func init() {
	daemonCmd.Flags().BoolVar(&testMode, "test", false, "Test the configuration without starting the daemon")
	rootCmd.AddCommand(daemonCmd)
}
