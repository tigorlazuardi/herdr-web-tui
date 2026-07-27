{
  lib,
  buildGoModule,
  buildNpmPackage,
  nix-gitignore,
}:
let
  src = nix-gitignore.gitignoreSource [ ] ../.;

  frontend = buildNpmPackage {
    pname = "herdr-web-tui-frontend";
    version = "0.2.1";
    src = ../frontend;
    npmDepsHash = "sha256-r9v19k5899W7qJ12qemtfjqb1yzPJtWHDtpIT0Ro+4c=";
    installPhase = ''
      runHook preInstall
      cp -r dist $out
      runHook postInstall
    '';
  };
in
buildGoModule {
  pname = "herdr-web-tui";
  version = "0.2.1";
  inherit src;

  vendorHash = "sha256-RZj/UHO9rNxPOa5Prd93mj5/U8Re5KOQaAF3suy+KBU=";

  # dist.go embeds frontend/dist; supply the prebuilt frontend so `go build`
  # (which runs //go:embed all:frontend/dist) has something to embed.
  preBuild = ''
    rm -rf frontend/dist
    cp -r ${frontend} frontend/dist
  '';

  subPackages = [ "cmd/herdr-web-tui" ];

  meta = {
    description = "Standalone web frontend for a running Herdr server";
    mainProgram = "herdr-web-tui";
    license = lib.licenses.mit;
  };
}
