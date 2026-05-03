{
  lib,
  buildGoModule,
  nodejs,
  pnpm,
  fetchPnpmDeps,
  pnpmConfigHook,
  stdenv,
}:

let
  frontend = stdenv.mkDerivation {
    pname = "foyer-frontend";
    version = "0.1.0";
    src = ../frontend;

    nativeBuildInputs = [
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

    buildPhase = ''
      runHook preBuild
      pnpm build
      runHook postBuild
    '';

    installPhase = ''
      runHook preInstall
      cp -r build $out
      runHook postInstall
    '';
  };
in
buildGoModule {
  pname = "foyer";
  version = "0.1.0";
  src = ../.;

  vendorHash = "sha256-1gFMZ5zn/xvLiNXY9MeSzFTdinG39mDT+TcPIbvnAuM=";

  preBuild = lib.optionalString stdenv.isLinux ''
    cp -r ${frontend} frontend/build
  '';

  subPackages =
    if stdenv.isLinux then
      [
        "."
        "./cmd/api"
        "./cmd/foyer-vm-controller"
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
