{ pkgs }:
let
  version = "0.2.0-261119-040e142";
  source = pkgs.fetchurl {
    url = "https://registry.npmjs.org/unkernel/-/unkernel-${version}.tgz";
    sha256 = "001642f481d94dfd441fc289b005939806e198b06d6ab2b27ff4e4e07d7ce161";
  };
  qemu = builtins.fromJSON (builtins.readFile ../cmd/course-vm-assets/qemu-assets.json);
  qemuFiles = map (asset: {
    inherit (asset) name;
    file = if asset ? source then ../cmd/course-vm-assets + "/${asset.source}"
      else pkgs.fetchurl { inherit (asset) url hash; };
  }) qemu;
in pkgs.runCommand "base58-course-vm-assets" {} ''
  tar -xzf ${source}
  mkdir -p $out/unkernel-${version}/_chunks
  cp package/dist/web.mjs package/dist/unkernel.wasm package/LICENSE package/LICENSE-MIT $out/unkernel-${version}/
  cp package/dist/_chunks/core.mjs $out/unkernel-${version}/_chunks/
  mkdir -p $out/qemu-20260913/guest
  ${pkgs.lib.concatMapStringsSep "\n" (asset: "cp ${asset.file} $out/qemu-20260913/${asset.name}") qemuFiles}
''
