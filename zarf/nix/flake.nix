{
  description = "Go Kronk workspace";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-unstable";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      gomod2nix,
    }:
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

          kronkBase = gomod2nix.legacyPackages.${system}.buildGoApplication {
            pname = "kronk";
            version =
              let
                src = builtins.readFile ../../sdk/kronk/kronk.go;
                lines = builtins.filter builtins.isString (builtins.split "\n" src);
                versionLine = builtins.head (
                  builtins.filter (l: builtins.match "const Version = .*" l != null) lines
                );
                match = builtins.match "const Version = \"([^\"]+)\"" versionLine;
              in
              builtins.head match;
            src = ../../.;
            subPackages = [ "cmd/kronk" ];
            modules = ./gomod2nix.toml;

            go = pkgs.go_1_26;
          };

          # Wrap kronk with the runtime libs needed for dynamic library loading.
          mkKronkPackage =
            {
              extraLibs ? [ ],
            }:
            pkgs.symlinkJoin {
              name = "kronk";
              paths = [ kronkBase ];
              nativeBuildInputs = [ pkgs.makeWrapper ];
              postBuild = ''
                wrapProgram $out/bin/kronk \
                  --prefix LD_LIBRARY_PATH : "${
                    pkgs.lib.makeLibraryPath (
                      [
                        pkgs.libffi
                        pkgs.stdenv.cc.cc.lib
                      ]
                      ++ extraLibs
                    )
                  }"
              '';
            };
        in
        {
          # nix build (defaults to cpu)
          default = self.packages.${system}.cpu;

          # nix build .#cpu
          cpu = mkKronkPackage { };

          # nix build .#vulkan
          vulkan = mkKronkPackage { extraLibs = [ pkgs.vulkan-loader ]; };

          # nix build .#cuda
          cuda = mkKronkPackage { };
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
            gomod2nix.legacyPackages.${system}.gomod2nix
          ];

          # Shared environment variables across all dev shells.
          baseLibs = [
            pkgs.libffi
            pkgs.stdenv.cc.cc.lib
          ];

          mkKronkShell =
            {
              extraPackages ? [ ],
              extraLibs ? [ ],
            }:
            pkgs.mkShell {
              buildInputs = basePackages ++ extraPackages;
              shellHook = ''
                gomod2nix import &> /dev/null
              '';

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
