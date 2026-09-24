{
  description = "Gogg: a multiplatform game file downloader for GOG";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          # What Fyne needs on Linux to build and run the desktop GUI.
          guiDeps = nixpkgs.lib.optionals pkgs.stdenv.isLinux (with pkgs; [
            libGL
            libxkbcommon
            xorg.libX11
            xorg.libXcursor
            xorg.libXi
            xorg.libXinerama
            xorg.libXrandr
            xorg.libXxf86vm
          ]);
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              # Go toolchain and build tools. (The sqlite driver uses cgo, so a C
              # compiler is part of the toolchain here.)
              go
              gcc
              gnumake
              pkg-config

              # Other related tools
              golangci-lint
              go-tools
              gofumpt
              gotools
              pre-commit
              uv
            ] ++ guiDeps;
          };
        }
      );

      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          # The CLI-only binary. The desktop GUI needs a display server and
          # OpenGL at build time, which is what the dev shell is for; the
          # packaged artifact is the one that makes sense to run anywhere.
          default = pkgs.buildGoModule {
            pname = "gogg";
            version = "0.5.2-beta";
            src = ./.;

            # The hash of what go mod vendor produces for go.mod and go.sum.
            # When the dependencies change, Nix build prints the new value to put here.
            vendorHash = "sha256-ikrEMyl8uQshhSHMkyXEhp0AXKaboNTQ0wESWG4LN7g=";

            tags = [ "headless" ];
            env.CGO_ENABLED = "1";

            # The test suite runs through make in the dev shell.
            doCheck = false;

            meta = with nixpkgs.lib; {
              description = "A downloader for GOG.com";
              homepage = "https://github.com/habedi/gogg";
              license = licenses.mit;
              mainProgram = "gogg";
            };
          };
        }
      );
    };
}
