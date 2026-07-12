{
  description = "herdr-web-tui — standalone web frontend for a running Herdr server";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEach = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forEach (pkgs: rec {
        herdr-web-tui = pkgs.callPackage ./nix/package.nix { };
        default = herdr-web-tui;
      });

      devShells = forEach (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.nodejs pkgs.gopls pkgs.gotools ];
        };
      });

      # Home Manager module. Resolves the herdr package from programs.herdr
      # when that module exists, else pkgs.herdr.
      homeManagerModules.default = import ./nix/home-manager-module.nix { inherit self; };
    };
}
