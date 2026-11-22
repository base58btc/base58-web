{
  description = "Base58 Taproot class";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { nixpkgs, ... }: {
    devShells = nixpkgs.lib.genAttrs [ "aarch64-linux" "x86_64-linux" "aarch64-darwin" ] (system:
      let pkgs = nixpkgs.legacyPackages.${system};
      in {
        default = pkgs.mkShellNoCC {
          packages = [
            pkgs.bashInteractive
            pkgs.coreutils
            pkgs.jq
            pkgs.bitcoind
            (pkgs.python3.withPackages (ps: [ ps.coincurve ]))
          ];
          shellHook = ''
            export BASE58_LAB="$PWD/.lab"
            export PYTHONDONTWRITEBYTECODE=1
          '';
        };
      });
  };
}
