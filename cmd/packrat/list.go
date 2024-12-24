package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/logandonley/packrat/pkg/storage"
)

type backupInfo struct {
	count  int
	latest *storage.BackupFile
}

func getServiceBackupInfo(store storage.Storage, serviceName string) (backupInfo, error) {
	if store == nil {
		return backupInfo{}, nil
	}

	backups, err := store.List(serviceName + "-")
	if err != nil {
		return backupInfo{}, err
	}

	if len(backups) == 0 {
		return backupInfo{}, nil
	}

	// Sort backups by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		timeI := parseBackupTime(backups[i].ModTime)
		timeJ := parseBackupTime(backups[j].ModTime)
		return timeI.After(timeJ)
	})

	return backupInfo{
		count:  len(backups),
		latest: &backups[0],
	}, nil
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all services and their backups",
	Long: `List all configured services and their backups, showing:
- Service name and path
- Docker container (if configured)
- Total number of backups
- Latest backup details for each storage backend`,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := createManager()
		if err != nil {
			return fmt.Errorf("failed to create backup manager: %w", err)
		}

		// Get list of services
		services := manager.GetConfig().Services

		// Print header
		fmt.Println("\nConfigured services and their backups:")
		fmt.Println(strings.Repeat("─", 100))

		// Process each service
		for serviceName, service := range services {
			fmt.Printf("\n📁 Service: %s\n", serviceName)
			fmt.Printf("   Path: %s\n", service.Path)
			if service.Docker != nil {
				fmt.Printf("   Docker container: %s\n", service.Docker.Container)
			}

			// Get Synology backup info
			synologyInfo, err := getServiceBackupInfo(manager.Synology, serviceName)
			if err != nil {
				return fmt.Errorf("failed to get Synology backup info for %s: %w", serviceName, err)
			}

			// Get S3 backup info
			s3Info, err := getServiceBackupInfo(manager.S3, serviceName)
			if err != nil {
				return fmt.Errorf("failed to get S3 backup info for %s: %w", serviceName, err)
			}

			// Print backup information
			fmt.Printf("\n   Backup summary:\n")

			// Synology info
			fmt.Printf("   ├─ Synology: %d backups\n", synologyInfo.count)
			if synologyInfo.latest != nil {
				backupTime := parseBackupTime(synologyInfo.latest.ModTime)
				fmt.Printf("   │  └─ Latest: %s (%s, %s)\n",
					synologyInfo.latest.Name,
					humanize.Time(backupTime),
					humanize.Bytes(uint64(synologyInfo.latest.Size)),
				)
			}

			// S3 info
			if manager.S3 != nil {
				fmt.Printf("   └─ S3: %d backups\n", s3Info.count)
				if s3Info.latest != nil {
					backupTime := parseBackupTime(s3Info.latest.ModTime)
					fmt.Printf("      └─ Latest: %s (%s, %s)\n",
						s3Info.latest.Name,
						humanize.Time(backupTime),
						humanize.Bytes(uint64(s3Info.latest.Size)),
					)
				}
			}
		}

		fmt.Println("\n" + strings.Repeat("─", 100))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
