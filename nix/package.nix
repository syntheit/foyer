{
  lib,
  buildGoModule,
  nodejs,
  pnpm,
  fetchPnpmDeps,
  pnpmConfigHook,
  stdenv,
}:

buildGoModule {
  pname = "foyer";
  version = "0.1.0";
  src = ../.;

  vendorHash = "sha256-1gFMZ5zn/xvLiNXY9MeSzFTdinG39mDT+TcPIbvnAuM=";

  nativeBuildInputs = lib.optionals stdenv.isLinux [
    nodejs
    pnpm
    pnpmConfigHook
  ];

  pnpmDeps = fetchPnpmDeps {
    pname = "foyer-frontend";
    src = ../frontend;
    hash = "sha256-dtIvWxOrkzkJe3mFlc9ACxdqqNmgqFCE1wv890Wmjlc=";
    fetcherVersion = 3;
  };

  pnpmRoot = "frontend";

  preBuild = lib.optionalString stdenv.isLinux ''
    pnpm --dir frontend build
  '';

  subPackages =
    if stdenv.isLinux then
      [
        "."
        "./cmd/api"
      ]
    else
      [ "./cmd/api" ];

  postInstall = ''
    mv $out/bin/api $out/bin/foyer-api
  '';

  ldflags = [
    "-s"
    "-w"
  ];

  meta = {
    description = "Self-hosted server dashboard";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
      "aarch64-darwin"
    ];
  };
}
