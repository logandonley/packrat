# NixOS module for Packrat
{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.packrat;
  
  # Helper function to read a secret file if specified
  getSecretValue = value: valueFile:
    if valueFile != null
    then "''${valueFile}"
    else value;

  # Helper to process s3 settings with potential secret files
  processS3Settings = s3:
    if s3 == null then null
    else s3 // {
      access_key_id = getSecretValue (s3.access_key_id or null) (s3.access_key_id_file or null);
      secret_access_key = getSecretValue (s3.secret_access_key or null) (s3.secret_access_key_file or null);
    };

  # Process all settings, handling s3 section specially
  processSettings = settings:
    if settings ? backup.s3
    then settings // { backup = settings.backup // { s3 = processS3Settings settings.backup.s3; }; }
    else settings;

in {
  options.services.packrat = {
    enable = mkEnableOption "Packrat backup service";

    package = mkOption {
      type = types.package;
      default = pkgs.packrat;  # Assuming packrat is in nixpkgs
      defaultText = literalExpression "pkgs.packrat";
      description = "The packrat package to use.";
    };

    user = mkOption {
      type = types.str;
      default = "packrat";
      description = "User account under which packrat runs.";
    };

    group = mkOption {
      type = types.str;
      default = "packrat";
      description = "Group under which packrat runs.";
    };

    settings = mkOption {
      type = types.attrs;
      default = {};
      example = literalExpression ''
        {
          encryption.key_file = "~/.config/packrat/key";
          services = {
            gitea = {
              path = "/var/lib/gitea";
              schedule = "0 2 * * *";
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
              access_key_id = "your-access-key";  # Optional if using access_key_id_file
              secret_access_key = "your-secret-key";  # Optional if using secret_access_key_file
              access_key_id_file = null;  # Path to file containing access key ID
              secret_access_key_file = null;  # Path to file containing secret access key
              path = "backups/packrat/";
            };
          };
        }
      '';
      description = "Packrat configuration. Will be converted to YAML. For S3 credentials, you can either specify them directly or use *_file options to load from files (e.g., sops-nix managed files).";
    };

    configDir = mkOption {
      type = types.str;
      default = "/var/lib/packrat";
      description = "Directory where packrat stores its configuration.";
    };
  };

  config = mkIf cfg.enable {
    # Make packrat available system-wide
    environment.systemPackages = [ cfg.package ];

    users.users.${cfg.user} = {
      isSystemUser = true;
      group = cfg.group;
      home = cfg.configDir;
      createHome = true;
      description = "Packrat backup service user";
    };

    users.groups.${cfg.group} = {};

    # Make config accessible to both the service and CLI operations
    environment.etc."packrat/config.yaml" = {
      source = pkgs.writeText "packrat-config.yaml"
        (builtins.toJSON (processSettings cfg.settings));
      # Make config readable by users in the packrat group
      mode = "0640";
      group = cfg.group;
    };

    # Create a group for users who need to use packrat CLI
    users.groups.packrat-users = {};

    systemd.services.packrat = {
      description = "Packrat Backup Service";
      documentation = [ "https://github.com/logandonley/packrat" ];
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" "docker.service" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;
        ExecStart = "${cfg.package}/bin/packrat daemon";
        Restart = "always";
        RestartSec = 10;
        TimeoutStopSec = 90;

        # Security hardening
        NoNewPrivileges = true;
        ProtectSystem = "full";
        ProtectHome = "read-only";
        PrivateTmp = true;
        ReadWritePaths = [ "/tmp" cfg.configDir ];
        CapabilityBoundingSet = [ "CAP_DAC_READ_SEARCH" "CAP_NET_RAW" ];
        AmbientCapabilities = [ "CAP_DAC_READ_SEARCH" "CAP_NET_RAW" ];

        # Environment
        Environment = [
          "HOME=${cfg.configDir}"
          "XDG_CONFIG_HOME=${cfg.configDir}/.config"
        ];
      };
    };
  };
} 