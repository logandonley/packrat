# Packrat

A friendly daemon and CLI tool for managing backups of homelab services to both s3-compatible storage and Synology NAS destinations.

## Overview

Packrat runs as a daemon to perform scheduled backups of specified directories and Docker containers, uploading encrypted backups to both s3-compatible storage and a Synology NAS. It also provides a CLI interface for manual operations and restores.

## Features

### Core Components
- Daemon mode for scheduled backups
- CLI interface for manual operations
- TUI interface for restore operations
- YAML-based service configuration

### Backup Capabilities
- Directory-based backups for services
- Docker container handling (stop -> backup -> start)
- Pre-backup command execution (e.g., database dumps)
- Compression of backup files
- Encryption of backups (AES-256)
- Separate backup file per service
- Multi-destination upload (B2 and Synology NAS)

### Encryption
- Initial setup prompts for password
- Uses Argon2 for key derivation
- Stores derived AES-256 key
- Can regenerate key from original password if lost
- Automatic encryption/decryption during backup/restore

## Configuration

Example configuration file:

```yaml
encryption:
  key_file: ~/.config/packrat/key  # Contains the derived encryption key

services:
  # Example of a Docker-based service
  gitea:
    path: /var/lib/gitea
    schedule: "0 2 * * *"  # 2 AM daily
    docker:
      container: gitea
    exclude:
      - "**/tmp/**"
      - "**/.git"
    retain_backups: 14  # Keep last 14 backups
  
  # Example of a service with pre-backup database dump
  postgres:
    path: /var/lib/postgres/backups
    schedule: "0 3 * * *"  # 3 AM daily
    docker:
      container: postgres
    pre_backup:
      command: pg_dumpall -U postgres > /var/lib/postgres/backups/dump.sql
      working_dir: /var/lib/postgres
      environment:
        PGPASSWORD: your-password
      timeout: 30m  # Allow up to 30 minutes for the dump
    exclude:
      - "**/*.log"
    retain_backups: 30  # Keep last 30 backups

  # Example of a simple directory backup without Docker
  documents:
    path: /home/user/documents
    schedule: "0 1 * * *"  # 1 AM daily
    exclude:
      - "**/.DS_Store"
      - "**/node_modules"
      - "**/*.tmp"
    retain_backups: 60  # Keep last 60 backups

  # Example of a non-Docker service with API-based backup
  nextcloud:
    path: /var/www/nextcloud/data/backups
    schedule: "0 4 * * *"  # 4 AM daily
    pre_backup:
      command: |
        curl -X POST \
          -H "Authorization: Bearer ${NC_TOKEN}" \
          -H "Content-Type: application/json" \
          http://localhost:8080/ocs/v2.php/apps/backup/api/v1/backup
      environment:
        NC_TOKEN: your-nextcloud-token
      timeout: 2h  # Allow up to 2 hours for large instances
    exclude:
      - "**/*.part"
      - "**/cache/*"
    retain_backups: 14

backup:
  retain_backups: 7  # Global default: keep last 7 backups
  synology:
    host: 192.168.1.100
    port: 22
    username: backups
    key_file: ~/.ssh/id_rsa
    path: ./backups/test/
  s3:
    # For Backblaze B2, use endpoint: https://s3.REGION.backblazeb2.com
    # For MinIO, use your MinIO server endpoint
    # For AWS S3, leave endpoint empty
    endpoint: https://s3.us-west-001.backblazeb2.com
    region: us-west-001
    bucket: homelab-backups
    access_key_id: your-access-key
    secret_access_key: your-secret-key
    path: backups/packrat/
```

## Usage

### Initial Setup

```bash
# Initialize the tool and set up encryption
packrat init
```

### Managing Backups

```bash
# Manual backup of a specific service
packrat backup gitea

# List available backups
packrat list

# Restore a backup (launches TUI)
packrat restore gitea
```

### Key Management

```bash
# Regenerate key from original password
packrat rekey
```

## Technical Details

### Encryption Process

1. During initialization:
   - User provides password
   - Argon2 KDF generates 32-byte key
   - Key is stored for daemon/CLI use

2. During backup:
   - Files are compressed
   - AES-256 encryption using stored key
   - Separate file per service

3. During restore:
   - Reads stored encryption key
   - Decrypts backup file
   - Decompresses to target location

### Docker Integration

For Docker-based services:
1. Stop container
2. Backup specified directory
3. Restart container
4. Verify container health

### Pre-Backup Commands

For services that require preparation before backup:
1. Execute pre-backup command with specified environment
   - Command runs from service path by default
   - Can override with explicit `working_dir`
2. Wait for command completion or timeout
3. Proceed with backup if command succeeds
4. Fail backup if command fails

Example with default working directory:
```yaml
services:
  postgres:
    path: /var/lib/postgres
    pre_backup:
      command: pg_dumpall -U postgres > backups/dump.sql
      environment:
        PGPASSWORD: your-password
      timeout: 30m
```

Example with explicit working directory:
```yaml
services:
  nextcloud:
    path: /var/www/nextcloud
    pre_backup:
      command: php occ maintenance:mode --on
      working_dir: /var/www/nextcloud/html  # Override default
      timeout: 1m
```
