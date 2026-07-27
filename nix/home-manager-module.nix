# Home Manager module: runs herdr-web-tui as a systemd user service.
{ self }:
{ config, options, lib, pkgs, ... }:
let
  cfg = config.services.herdr-web-tui;

  # Newer Home Manager ships a `programs.herdr` module. When present, enable it
  # and take the herdr package from there; otherwise fall back to pkgs.herdr.
  hasHerdrModule = options ? programs && options.programs ? herdr;
  herdrPkg =
    if cfg.herdrPackage != null then cfg.herdrPackage
    else if hasHerdrModule then config.programs.herdr.package
    else pkgs.herdr;

  serverUnit = "herdr-server.service";
in
{
  options.services.herdr-web-tui = {
    enable = lib.mkEnableOption "herdr-web-tui web frontend (user service)";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.system}.default;
      description = "herdr-web-tui package to run.";
    };

    herdrPackage = lib.mkOption {
      type = lib.types.nullOr lib.types.package;
      default = null;
      defaultText = lib.literalMD "`programs.herdr.package` if that module exists, else `pkgs.herdr`";
      description = ''
        The `herdr` package put on PATH; herdr-web-tui shells out to it.
        When null, uses `programs.herdr.package` if the herdr Home Manager
        module is available, otherwise `pkgs.herdr`.
      '';
    };

    server = {
      enable = lib.mkEnableOption ''
        a managed `herdr server` user service that herdr-web-tui depends on
        (PartOf/Requires/After). When disabled, no server dependency is wired
        and you must run the herdr server yourself'';
    };

    addr = lib.mkOption {
      type = lib.types.str;
      default = ":8080";
      description = "Listen address (ADDR).";
    };

    logFormat = lib.mkOption {
      type = lib.types.enum [ "" "json" "text" ];
      default = "json";
      description = "Log format (LOG_FORMAT).";
    };

    tmpPrefix = lib.mkOption {
      type = lib.types.str;
      default = "herdr-web-tui";
      description = "Prefix for the /tmp artifact staging dir (TMP_PREFIX).";
    };

    environment = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      description = ''
        Extra non-secret environment variables for the service.
        Never put `VAPID_PRIVATE_KEY` here: Nix values can leak into the store
        or generated unit. Use `webPush.environmentFile` instead.
      '';
    };

    webPush.environmentFile = lib.mkOption {
      type = lib.types.nullOr (lib.types.strMatching "^/.*");
      default = null;
      example = "/run/user/1000/secrets/herdr-web-push.env";
      description = ''
        Absolute runtime path to a systemd EnvironmentFile containing Web Push
        configuration, suitable for sops-nix or agenix output. The file is
        required when configured, so a missing or unreadable file prevents the
        service from starting. Keep VAPID private keys out of Nix values.
      '';
    };
  };

  config = lib.mkIf cfg.enable (lib.mkMerge [
    # Turn on the herdr HM module when it exists (newer Home Manager).
    (lib.mkIf hasHerdrModule { programs.herdr.enable = lib.mkDefault true; })

    {
      systemd.user.services.herdr-web-tui = {
        Unit = {
          Description = "herdr-web-tui web frontend";
          After = [ "network.target" ] ++ lib.optional cfg.server.enable serverUnit;
          # Only bind to the server when we manage it.
          Requires = lib.optional cfg.server.enable serverUnit;
          PartOf = lib.optional cfg.server.enable serverUnit;
        };
        Install.WantedBy = [ "default.target" ];
        Service = {
          ExecStart = lib.getExe cfg.package;
          Restart = "on-failure";
          Environment = lib.mapAttrsToList (n: v: "${n}=${v}") ({
            ADDR = cfg.addr;
            LOG_FORMAT = cfg.logFormat;
            TMP_PREFIX = cfg.tmpPrefix;
            PATH = "${herdrPkg}/bin";
          } // cfg.environment);
        } // lib.optionalAttrs (cfg.webPush.environmentFile != null) {
          EnvironmentFile = cfg.webPush.environmentFile;
        };
      };
    }

    # Managed herdr server user service (opt-in).
    (lib.mkIf cfg.server.enable {
      systemd.user.services.herdr-server = {
        Unit = {
          Description = "herdr headless server";
          After = [ "network.target" ];
        };
        Install.WantedBy = [ "default.target" ];
        Service = {
          ExecStart = "${lib.getExe herdrPkg} server";
          ExecStop = "${lib.getExe herdrPkg} server stop";
          Restart = "on-failure";
        };
      };
    })
  ]);
}
