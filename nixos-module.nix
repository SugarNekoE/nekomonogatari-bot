{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.nekomonogatari-bot;
in
{
  options.services.nekomonogatari-bot = {
    enable = lib.mkEnableOption "nekomonogatari Telegram bot";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./package.nix { };
      defaultText = lib.literalExpression "pkgs.callPackage ./package.nix { }";
      description = "The nekomonogatari-bot package to run.";
    };

    configPath = lib.mkOption {
      type = lib.types.str;
      example = "/run/secrets/nekomonogatari-bot.yaml";
      description = ''
        Absolute path to the bot's config.yaml. Because this file normally
        contains secrets, pass a runtime secret path rather than a Nix path
        literal, which would copy the configuration into the world-readable
        Nix store. The file must be readable by the nekomonogatari-bot user.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = lib.hasPrefix "/" cfg.configPath;
        message = "services.nekomonogatari-bot.configPath must be an absolute path";
      }
    ];

    users.groups.nekomonogatari-bot = { };
    users.users.nekomonogatari-bot = {
      isSystemUser = true;
      group = "nekomonogatari-bot";
      home = "/var/lib/nekomonogatari-bot";
    };

    systemd.services.nekomonogatari-bot = {
      description = "Nekomonogatari Telegram bot";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];

      environment.CONFIG_PATH = cfg.configPath;
      serviceConfig = {
        Type = "simple";
        User = "nekomonogatari-bot";
        Group = "nekomonogatari-bot";
        ExecStart = lib.getExe cfg.package;
        WorkingDirectory = "/var/lib/nekomonogatari-bot";
        StateDirectory = "nekomonogatari-bot";
        StateDirectoryMode = "0750";
        Restart = "on-failure";
        RestartSec = "5s";
        UMask = "0077";

        CapabilityBoundingSet = "";
        LockPersonality = true;
        NoNewPrivileges = true;
        PrivateDevices = true;
        PrivateTmp = true;
        ProtectClock = true;
        ProtectControlGroups = true;
        ProtectHome = true;
        ProtectHostname = true;
        ProtectKernelLogs = true;
        ProtectKernelModules = true;
        ProtectKernelTunables = true;
        ProtectSystem = "strict";
        RemoveIPC = true;
        RestrictAddressFamilies = [
          "AF_UNIX"
          "AF_INET"
          "AF_INET6"
        ];
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        SystemCallArchitectures = "native";
      };
    };
  };
}
