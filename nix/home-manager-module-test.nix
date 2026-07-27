# Runnable with: nix eval --file nix/home-manager-module-test.nix
let
  flake = builtins.getFlake (toString ../.);
  pkgs = flake.inputs.nixpkgs.legacyPackages.${builtins.currentSystem};
  lib = pkgs.lib;
  module = import ./home-manager-module.nix {
    self.packages.${pkgs.system}.default = pkgs.hello;
  };
  evaluate = environmentFile: lib.evalModules {
    specialArgs = { inherit pkgs; };
    modules = [
      ({ lib, ... }: {
        options = {
          programs = lib.mkOption {
            type = lib.types.attrsOf lib.types.anything;
            default = { };
          };
          systemd.user.services = lib.mkOption {
            type = lib.types.attrsOf (lib.types.submodule {
              freeformType = lib.types.attrsOf lib.types.anything;
            });
            default = { };
          };
        };
      })
      module
      {
        services.herdr-web-tui = {
          enable = true;
          package = pkgs.hello;
          herdrPackage = pkgs.hello;
          webPush.environmentFile = environmentFile;
        };
      }
    ];
  };
  disabled = (evaluate null).config.systemd.user.services.herdr-web-tui.Service;
  enabled = (evaluate "/run/user/1000/secrets/herdr-web-push.env").config.systemd.user.services.herdr-web-tui.Service;
  invalid = builtins.tryEval (builtins.deepSeq
    (evaluate "relative.env").config.systemd.user.services.herdr-web-tui.Service
    true);
in
assert !(disabled ? EnvironmentFile);
assert enabled.EnvironmentFile == "/run/user/1000/secrets/herdr-web-push.env";
assert !invalid.success;
true
