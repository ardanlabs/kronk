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

          pkgsCuda = import nixpkgs {
            inherit system;
            config = {
              allowUnfree = true;
              cudaSupport = true;
            };
          };

          # Base kronk CLI binary (CGO_ENABLED=0, no llama.cpp dependency).
          kronkBase = (pkgs.buildGoModule.override { go = pkgs.go_1_26; }) {
            pname = "kronk";
            version = "1.20.1";
            src = ../../.;
            subPackages = [ "cmd/kronk" ];
            vendorHash = "sha256-1sNrS2prNd7DnTU3pRw+IptsP4GISVSe5mKAe4BlytA=";

            env.CGO_ENABLED = 0;
          };

          # Helper to wrap the kronk binary with KRONK_LIB_PATH pointing
          # to the correct llama.cpp backend and runtime libraries.
          mkKronkPackage =
            { llamaPkg }:
            pkgs.symlinkJoin {
              name = "kronk";
              paths = [ kronkBase ];
              nativeBuildInputs = [ pkgs.makeWrapper ];
              postBuild = ''
                wrapProgram $out/bin/kronk \
                  --set KRONK_LIB_PATH "${llamaPkg}/lib" \
                  --set KRONK_ALLOW_UPGRADE "false" \
                  --prefix LD_LIBRARY_PATH : "${
                    pkgs.lib.makeLibraryPath [
                      pkgs.libffi
                      pkgs.stdenv.cc.cc.lib
                    ]
                  }"
              '';
            };
        in
        {
          # nix build (defaults to cpu)
          default = self.packages.${system}.cpu;

          # nix build .#cpu
          cpu = mkKronkPackage {
            llamaPkg = pkgs.llama-cpp;
          };

          # nix build .#vulkan
          vulkan = mkKronkPackage {
            llamaPkg = pkgs.llama-cpp-vulkan;
          };

          # nix build .#cuda
          cuda = mkKronkPackage {
            llamaPkg = pkgsCuda.llama-cpp;
          };
        }
      );

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
          baseEnv = {
            LD_LIBRARY_PATH = "${pkgs.lib.makeLibraryPath [
              pkgs.libffi
              pkgs.stdenv.cc.cc.lib
            ]}";
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
