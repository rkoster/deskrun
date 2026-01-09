{
  description = "DeskRun: Unlocking Local Compute for GitHub Actions";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        deskrun = pkgs.buildGoModule rec {
          pname = "deskrun";
          version = "0.1.0";

          src = ./.;

          vendorHash = "sha256-jz95iuStIb6m1OiBd9wLsDxDJh0NutCOadEXNPpMJ74=";

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
          ];

          subPackages = [ "cmd/deskrun" ];

          meta = with pkgs.lib; {
            description = "DeskRun: Unlocking Local Compute for GitHub Actions";
            homepage = "https://github.com/rkoster/deskrun";
            license = licenses.asl20;
            maintainers = [ ];
            mainProgram = "deskrun";
          };
        };

      in
      {
        packages = {
          default = deskrun;
          deskrun = deskrun;
        };

        apps = {
          default = {
            type = "app";
            program = "${deskrun}/bin/deskrun";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            kind
            kubectl
            docker
            gnumake
            git
          ];

          shellHook = ''
            echo "deskrun development environment"
            echo "Run 'make build' to build the project"
            echo "Run 'make test' to run tests"
          '';
        };
      }
    );
}
