package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/logandonley/packrat/pkg/backup"
	"github.com/logandonley/packrat/pkg/config"
	"github.com/robfig/cron/v3"
)

// Daemon handles the scheduling and execution of backups
type Daemon struct {
	config  *config.Config
	manager *backup.Manager
	cron    *cron.Cron
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// New creates a new daemon instance
func New(cfg *config.Config, manager *backup.Manager) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())
	return &Daemon{
		config:  cfg,
		manager: manager,
		cron:    cron.New(),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start initializes the daemon and starts scheduling backups
func (d *Daemon) Start() error {
	log.Println("Starting Packrat daemon...")

	// Schedule backups for each service
	for name, service := range d.config.Services {
		if service.Schedule == "" {
			log.Printf("Warning: Service %s has no schedule configured, skipping", name)
			continue
		}

		serviceName := name // Create a copy for the closure
		_, err := d.cron.AddFunc(service.Schedule, func() {
			log.Printf("Starting scheduled backup for service: %s", serviceName)
			if err := d.manager.CreateBackup(serviceName); err != nil {
				log.Printf("Error creating backup for service %s: %v", serviceName, err)
				return
			}
			log.Printf("Successfully completed backup for service: %s", serviceName)

			// Clean up old backups
			deletedCounts, err := d.manager.CleanupBackups(serviceName)
			if err != nil {
				log.Printf("Error cleaning up old backups for service %s: %v", serviceName, err)
				return
			}
			if count := deletedCounts[serviceName]; count > 0 {
				log.Printf("Cleaned up %d old backup(s) for service: %s", count, serviceName)
			}
		})

		if err != nil {
			return fmt.Errorf("failed to schedule backup for service %s: %w", name, err)
		}

		log.Printf("Scheduled backup for service %s with schedule: %s", name, service.Schedule)
	}

	// Start the cron scheduler
	d.cron.Start()
	log.Println("Packrat daemon started successfully")

	return nil
}

// Stop gracefully shuts down the daemon
func (d *Daemon) Stop() {
	log.Println("Stopping Packrat daemon...")
	d.cancel()
	<-d.cron.Stop().Done()
	d.wg.Wait()
	log.Println("Packrat daemon stopped")
}

// Run starts the daemon and blocks until Stop is called
func (d *Daemon) Run() error {
	if err := d.Start(); err != nil {
		return err
	}

	// Wait for context cancellation
	<-d.ctx.Done()
	return nil
}

// ParseCronSchedule validates a cron schedule expression
func ParseCronSchedule(schedule string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(schedule)
	return err
}
