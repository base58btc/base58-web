package courseenv

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImportedSourceChangeInvalidatesEnvironment(t *testing.T) {
	files := map[string]string{"flake.nix": "{}", "flake.lock": "{}", "nix/packages.nix": "old", "01_intro.md": "# Intro"}
	store := "/nix/store/" + strings.Repeat("a", 32) + "-bash"
	m := Manifest{Format: 1, System: System, SourceHash: SourceHash(files), Shell: store + "/bin/bash"}
	m.Archives = []Archive{{StorePath: store, Path: "runtime/" + m.SourceHash + "/" + strings.TrimPrefix(store, "/nix/store/") + ".nar.gz", SHA256: strings.Repeat("b", 64), Size: 1}}
	content, _ := json.Marshal(m)
	files[ManifestFile] = string(content)
	if _, err := ForFiles(files); err != nil {
		t.Fatal(err)
	}
	files["nix/packages.nix"] = "new"
	if _, err := ForFiles(files); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale environment, got %v", err)
	}
}

func TestManifestRejectsArchiveOutsideItsVersion(t *testing.T) {
	store := "/nix/store/" + strings.Repeat("a", 32) + "-bash"
	m := Manifest{Format: 1, System: System, SourceHash: strings.Repeat("b", 64), Shell: store + "/bin/bash", Archives: []Archive{{StorePath: store, Path: "../../other.nar.gz", SHA256: strings.Repeat("c", 64), Size: 1}}}
	content, _ := json.Marshal(m)
	if _, err := Parse(string(content)); err == nil {
		t.Fatal("accepted an archive outside the immutable version directory")
	}
}
