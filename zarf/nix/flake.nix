{
  description = "Go Kronk workspace";

  inputs.nixpkgs.url = "nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs supportedSystems (system: f system);
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = (pkgs.buildGoModule.override { go = pkgs.go_1_26; }) {
            pname = "kronk";
            version = "1.21.3";
            src = ../../.;
            subPackages = [ "cmd/kronk" ];
            vendorHash = "sha256-EF+wcHqrZV/zE9KdvJv353huhlbuPm7Na6cSizJl/dg=";

            env.CGO_ENABLED = 0;
          };
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          # Shared packages across all dev shells.
          basePackages = [
            pkgs.go_1_26
            pkgs.gopls
            pkgs.gotools
            pkgs.go-tools
            pkgs.pre-commit
            pkgs.pkg-config
            pkgs.typescript
            pkgs.vite
            pkgs.nodejs
          ];

          # Shared environment variables across all dev shells.
          baseLibs = [
            pkgs.libffi
            pkgs.stdenv.cc.cc.lib
          ];

          mkKronkShell =
            { extraPackages ? [ ], extraLibs ? [ ] }:
            pkgs.mkShell {
              buildInputs = basePackages ++ extraPackages;

              LD_LIBRARY_PATH = pkgs.lib.makeLibraryPath (baseLibs ++ extraLibs);
            };
        in
        {
          # nix develop (defaults to cpu)
          default = self.devShells.${system}.cpu;

          # nix develop .#cpu
          cpu = mkKronkShell { };

          # nix develop .#vulkan
          vulkan = mkKronkShell {
            extraPackages = [ pkgs.vulkan-headers ];
            extraLibs = [ pkgs.vulkan-loader ];
          };

          # nix develop .#cuda
          cuda = mkKronkShell { };
        }
      );
    };
}
