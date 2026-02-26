{
  description = "Go Kronk workspace";

  inputs.nixpkgs.url = "nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs supportedSystems (system: f system);
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          pkgsCuda = import nixpkgs {
            inherit system;
            config = {
              allowUnfree = true;
              cudaSupport = true;
            };
          };

          # Shared packages across all dev shells.
          basePackages = [
            pkgs.go_1_25
            pkgs.gopls
            pkgs.gotools
            pkgs.go-tools
            pkgs.pre-commit
            pkgs.pkg-config
            pkgs.typescript
            pkgs.vite
            pkgs.nodejs
            pkgs.libffi
            pkgs.gccNGPackages_15.libstdcxx
          ];

          # Shared environment variables across all dev shells.
          baseEnv = {
            LD_LIBRARY_PATH = "${pkgs.libffi}/lib:${pkgs.stdenv.cc.cc.lib}/lib:$LD_LIBRARY_PATH";
            KRONK_ALLOW_UPGRADE = "false";
          };

          # Helper to create a dev shell for a given llama.cpp package and
          # any extra packages it needs (e.g. vulkan headers/loader).
          mkKronkShell =
            {
              llamaPkg,
              extraPackages ? [ ],
            }:
            pkgs.mkShell {
              buildInputs = basePackages ++ [ llamaPkg ] ++ extraPackages;

              inherit (baseEnv) LD_LIBRARY_PATH KRONK_ALLOW_UPGRADE;
              KRONK_LIB_PATH = "${llamaPkg}/lib";
            };
        in
        {
          # nix develop (defaults to cpu)
          default = self.devShells.${system}.cpu;

          # nix develop .#cpu
          cpu = mkKronkShell {
            llamaPkg = pkgs.llama-cpp;
          };

          # nix develop .#vulkan
          vulkan = mkKronkShell {
            llamaPkg = pkgs.llama-cpp-vulkan;
            extraPackages = [
              pkgs.vulkan-headers
              pkgs.vulkan-loader
            ];
          };

          # nix develop .#cuda
          cuda = mkKronkShell {
            llamaPkg = pkgsCuda.llama-cpp;
          };
        }
      );
    };
}
