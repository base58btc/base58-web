package courseenv

import (
	"encoding/json"
	"path"
	"strings"
	"testing"
)

func preparedFiles(t *testing.T, format int) map[string]string {
	t.Helper()
	files := map[string]string{"flake.nix": "{}", "flake.lock": "{}", "nix/packages.nix": "old", "helper.py": "pass", "data/invoices.json": "[]", "01_intro.md": "# Intro", "lessons/02.MD": "# Next", "nix/build-notes.md": "package me"}
	store := "/nix/store/" + strings.Repeat("a", 32) + "-bash"
	m := Manifest{Format: format, System: System, Shell: store + "/bin/bash"}
	if format == 1 {
		m.SourceHash = ContentHash(files)
	} else {
		m.RuntimeHash = RuntimeHash(files)
	}
	m.Archives = []Archive{{StorePath: store, Path: "runtime/" + m.ArchiveHash() + "/" + strings.TrimPrefix(store, "/nix/store/") + ".nar.gz", SHA256: strings.Repeat("b", 64), Size: 1}}
	content, _ := json.Marshal(m)
	files[ManifestFile] = string(content)
	if _, err := ForFiles(files); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestLessonChangesReuseRuntimeButChangeContentIdentity(t *testing.T) {
	for _, name := range []string{"01_intro.md", "lessons/02.MD", "new/chapter.md"} {
		for _, remove := range []bool{false, true} {
			files := preparedFiles(t, 2)
			before := ContentHash(files)
			if remove {
				if _, exists := files[name]; !exists {
					continue
				}
				delete(files, name)
			} else {
				files[name] = "# Revised lesson\n+++ cell python-vm\nprint('new')\n+++"
			}
			if _, err := ForFiles(files); err != nil {
				t.Fatalf("lesson %s should reuse its runtime: %v", name, err)
			}
			if ContentHash(files) == before {
				t.Fatalf("lesson %s did not change the content identity", name)
			}
		}
	}
}

func TestRuntimeChangesRequireRebuild(t *testing.T) {
	for _, name := range []string{"flake.nix", "flake.lock", "nix/packages.nix", "helper.py", "data/invoices.json", "new.conf", "nix/build-notes.md"} {
		for _, remove := range []bool{false, true} {
			files := preparedFiles(t, 2)
			if remove {
				if _, exists := files[name]; !exists {
					continue
				}
				delete(files, name)
			} else {
				files[name] = "changed"
			}
			if _, err := ForFiles(files); err == nil {
				t.Fatalf("accepted changed runtime input %s (removed=%v)", name, remove)
			}
		}
	}
}

func TestLegacyBundlesKeepStrictValidationAndContentIdentity(t *testing.T) {
	legacy, current := preparedFiles(t, 1), preparedFiles(t, 2)
	if ContentHash(legacy) != ContentHash(current) {
		t.Fatal("manifest format upgrade changed the command-history identity")
	}
	legacy["01_intro.md"] = "# Changed"
	if _, err := ForFiles(legacy); err == nil {
		t.Fatal("legacy bundles must retain all-source validation")
	}
}

func TestManifestRejectsArchiveOutsideItsVersion(t *testing.T) {
	for _, format := range []int{1, 2} {
		files := preparedFiles(t, format)
		m, _ := ForFiles(files)
		for _, badPath := range []string{"../../other.nar.gz", "runtime/" + strings.Repeat("c", 64) + "/" + path.Base(m.Archives[0].StorePath) + ".nar.gz"} {
			m.Archives[0].Path = badPath
			content, _ := json.Marshal(m)
			if _, err := Parse(string(content)); err == nil {
				t.Fatal("accepted an archive outside the immutable runtime directory")
			}
		}
	}
}
