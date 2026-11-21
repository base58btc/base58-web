package handlers

import (
	"strings"
	"testing"
)

func TestBuildUploadedCourseSnapshotIncludesFlakeFiles(t *testing.T) {
	snapshot, err := buildUploadedCourseSnapshot("taproot", []CourseEditorSyncFile{
		{Path: "01_intro.md", Content: "# Intro\n"},
		{Path: "flake.nix", Content: "{ outputs = _: {}; }\n"},
		{Path: "flake.lock", Content: "{}\n"},
		{Path: "nix/packages.nix", Content: "{ pkgs }: [ pkgs.jq ]\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 4 {
		t.Fatalf("expected four source files, got %d", len(snapshot.Files))
	}
	if snapshot.Files[1].Path != courseLockFile || snapshot.Files[2].Path != courseFlakeFile {
		t.Fatalf("expected sorted flake files, got %#v", snapshot.Files)
	}
}

func TestBuildUploadedCourseSnapshotRequiresFlakePair(t *testing.T) {
	_, err := buildUploadedCourseSnapshot("taproot", []CourseEditorSyncFile{
		{Path: "01_intro.md", Content: "# Intro\n"},
		{Path: "flake.nix", Content: "{ outputs = _: {}; }\n"},
	})
	if err == nil || !strings.Contains(err.Error(), "both flake.nix and flake.lock") {
		t.Fatalf("expected missing lock error, got %v", err)
	}
}

func TestCleanUploadedCoursePathOnlyAllowsRootFlakeFiles(t *testing.T) {
	for _, path := range []string{"flake.nix", "flake.lock", "01_intro.md", "lessons/02_keys.md", "nix/packages.nix", "nested/flake.nix", "scripts/setup.sh"} {
		if _, ok := cleanUploadedCoursePath(path); !ok {
			t.Fatalf("expected %q to be accepted", path)
		}
	}
	for _, path := range []string{"flake.lock.bak", "secret.env", "binary.dat", ".secrets/config.json"} {
		if _, ok := cleanUploadedCoursePath(path); ok {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}
