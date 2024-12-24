# Running Packrat with Docker

This guide explains how to build and run Packrat using Docker.

## Building the Image

Build the Docker image locally:

```bash
docker build -t packrat .
```

## Running Packrat

### Prerequisites

Before running Packrat in Docker, you'll need:

1. A Packrat configuration file (`config.yaml`)
2. Your SSH private key for remote backups (if using SSH/SFTP)

### Basic Usage

Run the Packrat daemon with a mounted configuration and SSH key:

```bash
docker run -d \
  -v ~/.ssh/id_rsa:/root/.ssh/id_rsa:ro \
  -v /path/to/config.yaml:/root/.config/packrat/config.yaml:ro \
  packrat
```

### Explanation of Mounts

- `-v ~/.ssh/id_rsa:/root/.ssh/id_rsa:ro`: Mounts your SSH private key in read-only mode
- `-v /path/to/config.yaml:/root/.config/packrat/config.yaml:ro`: Mounts your Packrat configuration file in read-only mode

### Running Other Commands

To run other Packrat commands instead of the daemon:

```bash
# List backups
docker run --rm \
  -v ~/.ssh/id_rsa:/root/.ssh/id_rsa:ro \
  -v /path/to/config.yaml:/root/.config/packrat/config.yaml:ro \
  packrat list

# Show version
docker run --rm packrat version
```

### Security Considerations

1. Always use `:ro` (read-only) for mounted sensitive files
2. Consider using Docker secrets for sensitive information in production environments
3. Use specific SSH keys for Packrat rather than sharing your personal SSH key

### Example Configuration

Here's a minimal `config.yaml` example for use with Docker:

```yaml
backup:
  - name: "example"
    source: "/data/to/backup"
    destination: "sftp://user@host:/path/to/backup"
    schedule: "0 0 * * *"  # Daily at midnight
```

Remember to mount any source directories you want to backup:

```bash
docker run -d \
  -v ~/.ssh/id_rsa:/root/.ssh/id_rsa:ro \
  -v /path/to/config.yaml:/root/.config/packrat/config.yaml:ro \
  -v /data/to/backup:/data/to/backup:ro \
  packrat
``` 