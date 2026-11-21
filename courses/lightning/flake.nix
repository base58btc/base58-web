{
  description = "Base58 Lightning workshop: build once, run in the browser";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-linux" "x86_64-linux" "aarch64-darwin" ];
      forSystems = nixpkgs.lib.genAttrs systems;
    in {
      devShells = forSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShellNoCC {
            packages = [
              pkgs.bashInteractive pkgs.coreutils pkgs.jq
              pkgs.bitcoind pkgs.clightning
              (pkgs.python3.withPackages (ps: [ ps.coincurve ]))
            ];
            # Services start explicitly in lesson cells, so first boot is quick
            # and starting a daemon is itself part of the saved command history.
            shellHook = ''
              export BASE58_LAB="$PWD/.lab"
              export PYTHONDONTWRITEBYTECODE=1
            '';
          };
        });
    };
}
