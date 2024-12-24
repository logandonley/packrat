package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Encryption struct {
		KeyFile string `yaml:"key_file" mapstructure:"key_file"`
	} `yaml:"encryption" mapstructure:"encryption"`

	Services map[string]Service `yaml:"services" mapstructure:"services"`

	Backup BackupConfiguration `yaml:"backup" mapstructure:"backup"`
}

// Service represents a service to be backed up
type Service struct {
	Path          string   `yaml:"path" mapstructure:"path"`
	Schedule      string   `yaml:"schedule" mapstructure:"schedule"`
	Docker        *Docker  `yaml:"docker,omitempty" mapstructure:"docker,omitempty"`
	Exclude       []string `yaml:"exclude,omitempty" mapstructure:"exclude,omitempty"`
	RetainBackups *int     `yaml:"retain_backups,omitempty" mapstructure:"retain_backups,omitempty"`
	PreBackup     *Command `yaml:"pre_backup,omitempty" mapstructure:"pre_backup,omitempty"`
}

// Docker represents Docker-specific configuration
type Docker struct {
	Container string `yaml:"container" mapstructure:"container"`
}

// Command represents a command to be executed
type Command struct {
	Command     string            `yaml:"command" mapstructure:"command"`
	WorkingDir  string            `yaml:"working_dir,omitempty" mapstructure:"working_dir,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" mapstructure:"environment,omitempty"`
	Timeout     string            `yaml:"timeout,omitempty" mapstructure:"timeout,omitempty"`
}

// BackupConfiguration represents backup-specific settings
type BackupConfiguration struct {
	RetainBackups int      `yaml:"retain_backups" mapstructure:"retain_backups"`
	Synology      Synology `yaml:"synology" mapstructure:"synology"`
	S3            S3Config `yaml:"s3" mapstructure:"s3"`
}

// Synology represents Synology NAS configuration
type Synology struct {
	Host     string `yaml:"host" mapstructure:"host"`
	Port     int    `yaml:"port" mapstructure:"port"`
	Username string `yaml:"username" mapstructure:"username"`
	KeyFile  string `yaml:"key_file" mapstructure:"key_file"`
	Path     string `yaml:"path" mapstructure:"path"`
}

// S3Config represents S3-compatible storage configuration
type S3Config struct {
	Endpoint        string `yaml:"endpoint" mapstructure:"endpoint"`
	Region          string `yaml:"region" mapstructure:"region"`
	Bucket          string `yaml:"bucket" mapstructure:"bucket"`
	AccessKeyID     string `yaml:"access_key_id" mapstructure:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" mapstructure:"secret_access_key"`
	Path            string `yaml:"path" mapstructure:"path"`
}

// LoadConfig loads the configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
