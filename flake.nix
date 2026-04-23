{
  description = "Foyer — self-hosted server dashboard";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.callPackage ./nix/package.nix { };
        }
      );

      overlays.default = final: _prev: {
        foyer = self.packages.${final.stdenv.hostPlatform.system}.default;
      };

      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
              gotools
              nodejs_22
              pnpm
              sqlite
              just
            ];

            shellHook = ''
              echo "foyer dev shell"
              echo "  Go $(go version | cut -d' ' -f3) · Node $(node --version) · pnpm $(pnpm --version)"
            '';
          };
        }
      );
    };
}
