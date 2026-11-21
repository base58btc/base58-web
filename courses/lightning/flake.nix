{
  description = "Base58 Lightning class";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-linux"
        "x86_64-linux"
        "aarch64-darwin"
      ];
      forSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          pyln_bolt7 = pkgs.python3Packages.buildPythonPackage rec {
            pname = "pyln_bolt7";
            version = "cd894663";
            src = pkgs.fetchFromGitHub {
              owner = "niftynei";
              repo = "${pname}";
              rev = "${version}";
              sha256 = "sha256-//XG8aF2mW5DX0sBsAV1bL+9RLrvUXYpPSX9bz5f/OU=";
            };
            format = "setuptools";
            nativeBuildInputs = [ pkgs.python3Packages.setuptools ];
            doCheck = false;
            propagatedBuildInputs = [ ];
          };
          pyln_proto = pkgs.python3Packages.buildPythonPackage rec {
            pname = "pyln_proto";
            version = "87643bed";
            src = pkgs.fetchFromGitHub {
              owner = "niftynei";
              repo = "${pname}";
              rev = "${version}";
              sha256 = "sha256-q8Qh39e23C0jyerRlfobArKwWB9Zj3ghFS479oxcep8=";
            };
            format = "setuptools";
            nativeBuildInputs = [ pkgs.python3Packages.setuptools ];
            doCheck = false;
            propagatedBuildInputs = with pkgs.python3Packages; [
              coincurve
              cryptography
              pysocks
              bitstring
              base58
            ];
          };
          pyln_client = pkgs.python3Packages.buildPythonPackage rec {
            pname = "pyln_client";
            version = "23.5.2";
            src = pkgs.fetchFromGitHub {
              inherit version;
              owner = "niftynei";
              repo = "${pname}";
              rev = "250b8a2";
              sha256 = "sha256-vhGyBA5C5bgi5nMHgs9hjIUGOOKTwV31/OeBnQJUaL0=";
            };
            format = "setuptools";
            nativeBuildInputs = [ pkgs.python3Packages.setuptools ];
            doCheck = false;
            propagatedBuildInputs = [
              pyln_proto
              pyln_bolt7
            ];
          };
          gossipStore = pkgs.fetchurl {
            url = "https://media.githubusercontent.com/media/base58btc/lightning_class/62491b76177123c8cebcc0fadccc93ca826d8dc7/gossip_stores/nov_17.gossip_store";
            sha256 = "8db52d5a9b4ac8c637bafd82447a84c74fc7c866ca3c73ae3fdce817c609e8cd";
          };
          python = pkgs.python3.withPackages (ps: [
            ps.coincurve
            pyln_client
          ]);
          gossipIndex = pkgs.runCommand "lightning-gossip-index" { } ''
            ${python}/bin/python3 ${./gossip.py} ${gossipStore} "$out"
          '';
        in
        {
          default = pkgs.mkShellNoCC {
            packages = [
              pkgs.bashInteractive
              pkgs.coreutils
              pkgs.jq
              pkgs.gawk
              pkgs.gnugrep
              pkgs.gnused
              pkgs.less
              pkgs.bitcoind
              pkgs.clightning
              python
            ];
            # Services start explicitly in lesson cells, so first boot is quick
            # and starting a daemon is itself part of the saved command history.
            shellHook = ''
              export BASE58_LAB="$PWD/.lab"
              export PYTHONDONTWRITEBYTECODE=1
              export BITCOIN_BIN_DIR="${pkgs.bitcoind}/bin"
              export LIGHTNING_BIN_DIR="${pkgs.clightning}/bin"
              export PATH_TO_BITCOIN="$BASE58_LAB/bitcoin"
              export PATH_TO_LIGHTNING="$BASE58_LAB/nodes"
              export LIGHTNING_GOSSIP_STORE="${gossipStore}"
              export LIGHTNING_GOSSIP_INDEX="${gossipIndex}"
            '';
          };
        }
      );
    };
}
