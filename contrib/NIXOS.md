# Running Packrat on NixOS

This guide explains how to set up and run Packrat on NixOS using the provided NixOS module.

## Quick Start

1. Add the Packrat module to your NixOS configuration:

```nix
# /etc/nixos/configuration.nix
{ config, pkgs, ... }:

{
  imports = [
    # ... your other imports ...
    ./packrat.nix  # Path to the packrat.nix module
  ];

  services.packrat = {
    enable = true;
    settings = {
      encryption.key_file = "~/.config/packrat/key";
      services = {
        gitea = {
          path = "/var/lib/gitea";
          schedule = "0 2 * * *";  # 2 AM daily
          docker.container = "gitea";
          exclude = [ "**/tmp/**" "**/.git" ];
          retain_backups = 14;
        };
      };
      backup = {
        retain_backups = 7;
        synology = {
          host = "192.168.1.100";
          port = 22;
          username = "backups";
          key_file = "~/.ssh/id_rsa";
          path = "./backups/";
        };
        s3 = {
          # For Backblaze B2, use endpoint: https://s3.REGION.backblazeb2.com
          endpoint = "https://s3.us-west-001.backblazeb2.com";
          region = "us-west-001";
          bucket = "homelab-backups";
          # You can either specify credentials directly:
          access_key_id = "your-access-key";
          secret_access_key = "your-secret-key";
          # Or use files (e.g., from sops-nix):
          # access_key_id_file = config.sops.secrets."packrat/s3_access_key".path;
          # secret_access_key_file = config.sops.secrets."packrat/s3_secret_key".path;
          path = "backups/packrat/";
        };
      };
    };
  };

  # Optional: Add users who need to use packrat CLI to the packrat group
  users.users.youruser.extraGroups = [ "packrat" ];
}
```

2. Switch to the new configuration:

```bash
sudo nixos-rebuild switch
```

## Using the CLI

When you enable the Packrat service:
- The `packrat` binary is installed system-wide
- A shared configuration is created at `/etc/packrat/config.yaml`
- Users in the `packrat` group can access the configuration and run CLI commands

Common CLI commands:
```bash
# List available backups
packrat list

# Manually trigger a backup
packrat backup gitea

# Restore a backup (launches TUI)
packrat restore gitea
```

## Module Options

### Basic Options

- `enable`: Enable the Packrat service
- `package`: The Packrat package to use
- `user`: User to run Packrat as (default: "packrat")
- `group`: Group to run Packrat as (default: "packrat")
- `configDir`: Directory for Packrat configuration (default: "/var/lib/packrat")

### Configuration Settings

The `settings` option accepts a Nix attribute set that will be converted to YAML. All Packrat configuration options are supported:

```nix
settings = {
  encryption.key_file = "~/.config/packrat/key";
  services.<name> = {
    path = "/path/to/backup";
    schedule = "cron-expression";
    docker.container = "container-name";  # Optional
    exclude = [ "patterns" ];  # Optional
    retain_backups = 14;  # Optional
  };
  backup = {
    retain_backups = 7;
    synology = {
      host = "hostname";
      port = 22;
      username = "user";
      key_file = "~/.ssh/key";
      path = "./backups/";
    };
    s3 = {
      endpoint = "https://s3.us-west-001.backblazeb2.com";
      region = "us-west-001";
      bucket = "homelab-backups";
      # Credentials can be specified directly:
      access_key_id = "your-access-key";
      secret_access_key = "your-secret-key";
      # Or loaded from files:
      access_key_id_file = "/run/secrets/s3_access_key";
      secret_access_key_file = "/run/secrets/s3_secret_key";
      path = "backups/packrat/";
    };
  };
};
```

### Secret Management with sops-nix

The module supports loading S3 credentials from files, which works well with sops-nix. Here's how to set it up:

1. Set up sops-nix in your configuration:

```nix
{ config, pkgs, ... }:

{
  imports = [
    # ... other imports ...
    ./packrat.nix
  ];

  # Enable sops
  sops.defaultSopsFile = ./secrets.yaml;
  sops.age.keyFile = "/var/lib/sops-nix/key.txt";

  # Define your secrets
  sops.secrets."packrat/s3_access_key" = {
    owner = config.services.packrat.user;
    group = config.services.packrat.group;
    mode = "0440";  # Readable by owner and group
  };
  sops.secrets."packrat/s3_secret_key" = {
    owner = config.services.packrat.user;
    group = config.services.packrat.group;
    mode = "0440";  # Readable by owner and group
  };

  services.packrat = {
    enable = true;
    settings = {
      # ... rest of your settings ...
      backup.s3 = {
        endpoint = "https://s3.us-west-001.backblazeb2.com";
        region = "us-west-001";
        bucket = "homelab-backups";
        # Load credentials from sops-managed files
        access_key_id_file = config.sops.secrets."packrat/s3_access_key".path;
        secret_access_key_file = config.sops.secrets."packrat/s3_secret_key".path;
        path = "backups/packrat/";
      };
    };
  };
}
```

2. Create your `secrets.yaml` file with sops:

```yaml
packrat:
    s3_access_key: ENC[AES256_GCM,data:...] # Your encrypted access key
    s3_secret_key: ENC[AES256_GCM,data:...] # Your encrypted secret key
```

The module will automatically use the values from these files in the final configuration.

## Security Considerations

The NixOS module includes several security hardening measures:

- Runs as an unprivileged system user
- Read-only access to most of the system
- Write access only to `/tmp` and the config directory
- Limited capabilities for backup operations
- Protected home directory
- Support for loading sensitive values from files
- Configuration file permissions restricted to packrat group members

### Managing User Access

To allow a user to use the packrat CLI:

```nix
{
  users.users.youruser.extraGroups = [ "packrat" ];
}
```

This gives the user access to:
- Read the configuration file
- Execute packrat commands
- View backup status and history
- Perform manual backups and restores

## Troubleshooting

1. Check service status:
```bash
systemctl status packrat
```

2. View logs:
```bash
journalctl -u packrat -f
```

3. Test configuration:
```bash
sudo -u packrat packrat daemon --test
```

4. Common issues:

   - **Permission denied**: Check that the service user has access to the backup paths
   - **Docker access**: Ensure the service user is in the `docker` group
   - **SSH key access**: Verify SSH key permissions and ownership
   - **Secret access**: Verify that the service user has permission to read any secret files
   - **CLI access denied**: Ensure the user is in the `packrat` group

## Building from Source

If you want to build Packrat from source in your NixOS configuration:

```nix
{ config, pkgs, ... }:

let
  packrat = pkgs.buildGoModule {
    pname = "packrat";
    version = "0.1.0";
    src = pkgs.fetchFromGitHub {
      owner = "logandonley";
      repo = "packrat";
      rev = "main";  # Or specific version tag
      sha256 = "...";  # Use nix-prefetch-git to get this
    };
    vendorSha256 = "...";  # Use nix-prefetch to get this
  };
in {
  services.packrat = {
    enable = true;
    package = packrat;
    # ... rest of your configuration ...
  };
}
``` 