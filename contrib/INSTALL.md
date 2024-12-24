# Installing Packrat as a System Service

This guide explains how to set up Packrat as a systemd service.

## Prerequisites

1. Packrat binary installed at `/usr/local/bin/packrat`
2. Root or sudo access

## Installation Steps

1. Create packrat user and group:
```bash
sudo useradd -r -s /bin/false packrat
```

2. Create necessary directories and configuration:
```bash
# Create config directory
sudo mkdir -p /home/packrat/.config/packrat
sudo chown -R packrat:packrat /home/packrat

# Copy your configuration
sudo cp ~/.config/packrat/config.yaml /home/packrat/.config/packrat/
sudo chown packrat:packrat /home/packrat/.config/packrat/config.yaml
sudo chmod 600 /home/packrat/.config/packrat/config.yaml
```

3. Copy the systemd service file:
```bash
sudo cp contrib/packrat.service /etc/systemd/system/
```

4. Reload systemd daemon:
```bash
sudo systemctl daemon-reload
```

5. Start and enable the service:
```bash
sudo systemctl enable packrat
sudo systemctl start packrat
```

## Verifying Installation

Check the service status:
```bash
sudo systemctl status packrat
```

View logs:
```bash
sudo journalctl -u packrat -f
```

## Configuration

The default systemd service assumes:
- Binary location: `/usr/local/bin/packrat`
- Config directory: `/home/packrat/.config/packrat`
- Config file: `/home/packrat/.config/packrat/config.yaml`

Adjust the service file if your setup differs.

## Security Notes

The service runs with restricted privileges:
- Runs as unprivileged `packrat` user
- Read-only access to most of the system
- Write access only to `/tmp`
- Limited capabilities for backup operations

## Troubleshooting

1. If the service fails to start, check the logs:
```bash
sudo journalctl -u packrat -n 50 --no-pager
```

2. Verify permissions:
```bash
ls -la /home/packrat/.config/packrat
```

3. Test the configuration:
```bash
sudo -u packrat /usr/local/bin/packrat daemon --test
``` 