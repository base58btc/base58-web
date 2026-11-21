{
  description = "base58 website";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        courseVMAssets = import ./nix/course-vm-assets.nix { inherit pkgs; };
        buildBase58Command = name: subPackage:
          let binaryName = builtins.baseNameOf subPackage;
          in pkgs.buildGoModule {
          pname = name;
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-sXeQm6N4W5+xdtAzX3NqO/7fx/W6inVOVwPAnrryS2I=";
          subPackages = [ subPackage ];
          nativeBuildInputs = pkgs.lib.optionals (name == "base58-website") [ pkgs.makeWrapper ];
          postInstall = pkgs.lib.optionalString (binaryName != name) ''
            mv "$out/bin/${binaryName}" "$out/bin/${name}"
          '' + pkgs.lib.optionalString (name == "base58-website") ''
            mkdir -p "$out/share/base58-website"
            cp -R templates static courses "$out/share/base58-website/"
            mkdir -p "$out/share/base58-website/static/vendor"
            cp -R ${courseVMAssets}/* "$out/share/base58-website/static/vendor/"
            wrapProgram "$out/bin/base58-website" --chdir "$out/share/base58-website"
          '';
        };
      in
      {
        packages = {
          web = buildBase58Command "base58-website" "cmd/web";
          course-sync = buildBase58Command "course-sync" "cmd/course-sync";
          course-vm-assets = courseVMAssets;
          default = self.packages.${system}.web;
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            bashInteractive
            git
            jq
            nodejs_24
            python3
            go_1_25
            air 
            postgresql_16
          ];
          shellHook = ''
            export PGDATA="''${PGDATA:-$PWD/.nix-postgres/data}"
            export PGHOST="''${PGHOST:-127.0.0.1}"
            export PGPORT="''${PGPORT:-55432}"
            export PGUSER="''${PGUSER:-base58}"
            export PGDATABASE="''${PGDATABASE:-base58_dev}"
            export DB_DRIVER="''${DB_DRIVER:-postgres}"
            export DATABASE_URL="''${DATABASE_URL:-postgres://$PGUSER@$PGHOST:$PGPORT/$PGDATABASE?sslmode=disable}"
          '';
        };
      });
}
