{
  description = "base58 website";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            bashInteractive
            git
            jq
            go
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
