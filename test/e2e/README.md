# End-to-End Tests for Packrat

This directory contains end-to-end tests that validate the complete functionality of Packrat in a real environment.

## Prerequisites

1. Docker daemon running and accessible
2. Access to a Synology NAS (will use your existing configuration)
3. Go 1.21 or later

## What's Tested

The E2E test suite exercises:

1. **Configuration**
   - Creates an isolated test configuration
   - Validates Docker and Synology connectivity

2. **Docker Integration**
   - Creates a test nginx container
   - Tests container stop/start during backup
   - Verifies container health after operations

3. **Backup Operations**
   - Creates manual backups
   - Verifies backup content
   - Tests backup retention policies

4. **Restore Operations**
   - Restores from backup
   - Verifies restored content integrity
   - Tests Docker container handling during restore

5. **Daemon Operation**
   - Runs daemon with 1-minute schedule
   - Verifies automatic backup creation
   - Tests graceful shutdown

6. **Cleanup**
   - Tests backup retention policies
   - Verifies old backup removal
   - Cleans up test environment

## Running the Tests

### Full Test Suite

```bash
go test -v ./test/e2e
```

### Skip E2E Tests

```bash
go test -short ./test/e2e
```

## Test Environment

The test suite:
- Creates a temporary directory for all test files
- Uses an isolated config directory
- Creates a dedicated directory on your Synology NAS
- Cleans up all resources after completion

## Test Configuration

The test uses your existing Synology configuration (host, credentials) but:
- Creates its own test directory on the NAS
- Uses a separate test configuration file
- Does not interfere with your existing backups

## Cleanup

The test suite automatically cleans up:
- Local temporary files
- Docker containers
- Test backups on Synology

If a test fails, you might need to manually clean up:
1. Docker container: `docker rm -f packrat-test-nginx`
2. Synology directory: `./packrat-e2e-test/`
3. Local temp directory (printed in test output)

## Troubleshooting

1. **Docker Issues**
   - Ensure Docker daemon is running
   - Check Docker socket permissions
   - Verify nginx:alpine image is available

2. **Synology Issues**
   - Verify NAS is accessible
   - Check SSH key permissions
   - Ensure sufficient disk space

3. **Test Timeouts**
   - Default timeout is 2 minutes
   - Adjust timeouts in code if needed
   - Check network connectivity 