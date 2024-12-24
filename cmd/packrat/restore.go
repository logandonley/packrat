package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/logandonley/packrat/pkg/storage"
)

type backupWithSource struct {
	storage.BackupFile
	source string
}

func parseBackupTime(timeStr string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05 UTC", timeStr)
	if err != nil {
		return time.Time{}
	}
	return t
}

func displayBackupList(backups []backupWithSource) {
	// Sort backups by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		timeI := parseBackupTime(backups[i].ModTime)
		timeJ := parseBackupTime(backups[j].ModTime)
		return timeI.After(timeJ)
	})

	fmt.Println("\nAvailable backups:")
	fmt.Println(strings.Repeat("─", 100))
	fmt.Printf("%-3s %-30s %-15s %-15s %s\n", "#", "NAME", "SIZE", "SOURCE", "BACKED UP")
	fmt.Println(strings.Repeat("─", 100))

	for i, b := range backups {
		// Parse the backup time
		backupTime := parseBackupTime(b.ModTime)

		// Format the time as relative (e.g., "2 hours ago")
		timeAgo := humanize.Time(backupTime)

		// Format the size in human-readable format
		size := humanize.Bytes(uint64(b.Size))

		fmt.Printf("%-3d %-30s %-15s %-15s %s\n",
			i+1,
			b.Name,
			size,
			b.source,
			timeAgo,
		)
	}
	fmt.Println(strings.Repeat("─", 100))
}

var restoreCmd = &cobra.Command{
	Use:   "restore [service]",
	Short: "Restore a backup for a service",
	Long: `Restore a backup for a specified service. The backup will be downloaded from the selected storage backend,
decrypted, and extracted to the service's path. If the service uses a Docker container,
it will be stopped before restoration and started afterward.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]

		manager, err := createManager()
		if err != nil {
			return fmt.Errorf("failed to create backup manager: %w", err)
		}

		// Get list of backups from all storage backends
		var allBackups []backupWithSource

		// Get Synology backups
		synologyBackups, err := manager.Synology.List(serviceName + "-")
		if err != nil {
			return fmt.Errorf("failed to list Synology backups: %w", err)
		}
		for _, b := range synologyBackups {
			allBackups = append(allBackups, backupWithSource{
				BackupFile: b,
				source:     "synology",
			})
		}

		// Get S3 backups if configured
		if manager.S3 != nil {
			s3Backups, err := manager.S3.List(serviceName + "-")
			if err != nil {
				return fmt.Errorf("failed to list S3 backups: %w", err)
			}
			for _, b := range s3Backups {
				allBackups = append(allBackups, backupWithSource{
					BackupFile: b,
					source:     "s3",
				})
			}
		}

		if len(allBackups) == 0 {
			return fmt.Errorf("no backups found for service %s", serviceName)
		}

		// Display the list of backups
		displayBackupList(allBackups)

		// Get user selection
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nEnter the number of the backup to restore (or 'q' to quit): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if input == "q" || input == "Q" {
			return nil
		}

		// Parse the selection
		index, err := strconv.Atoi(input)
		if err != nil || index < 1 || index > len(allBackups) {
			return fmt.Errorf("invalid selection: %s", input)
		}

		// Get the selected backup
		selectedBackup := allBackups[index-1]

		// Show backup details
		backupTime := parseBackupTime(selectedBackup.ModTime)
		fmt.Printf("\nSelected backup details:\n")
		fmt.Printf("  Name: %s\n", selectedBackup.Name)
		fmt.Printf("  Size: %s\n", humanize.Bytes(uint64(selectedBackup.Size)))
		fmt.Printf("  Source: %s\n", selectedBackup.source)
		fmt.Printf("  Created: %s (%s)\n", backupTime.Format("Mon Jan 2 15:04:05 2006"), humanize.Time(backupTime))

		// Get the service configuration
		service, ok := manager.GetConfig().Services[serviceName]
		if !ok {
			return fmt.Errorf("service %s not found", serviceName)
		}

		// Handle Docker container if specified
		if service.Docker != nil {
			if err := manager.ValidateDockerContainer(service.Docker.Container); err != nil {
				return fmt.Errorf("failed to validate Docker container: %w", err)
			}
			fmt.Printf("\nDocker container %s will be stopped during restore and started afterward.\n", service.Docker.Container)
		}

		// Confirm the restore
		fmt.Print("\nAre you sure you want to restore this backup? (y/N): ")
		input, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if input != "y" && input != "Y" {
			fmt.Println("Restore cancelled.")
			return nil
		}

		fmt.Println("\nRestoring backup...")
		if err := manager.RestoreBackup(serviceName, selectedBackup.Name); err != nil {
			return fmt.Errorf("failed to restore backup: %w", err)
		}

		fmt.Printf("\nSuccessfully restored backup %s for service %s from %s\n", selectedBackup.Name, serviceName, selectedBackup.source)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
}
