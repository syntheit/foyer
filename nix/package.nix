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
      hash = "sha256-JCVcnAQBlnrqU5pBOG47dD6WPiBachMV4OEoJ4z/lGo=";
      fetcherVersion = 4;
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

  vendorHash = "sha256-3Rc8uI+BLwS2/QO3FTRrR094KN+2BMrozlvoUiRSDvI=";

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
