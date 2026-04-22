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
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
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

      nixosModules.default = import ./nix/module.nix;
    };
}
