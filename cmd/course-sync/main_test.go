package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCourseAssetsFindsRelativeMarkdownImages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fruit_market.png"), []byte("fruit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "diagrams"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diagrams", "field.png"), []byte("field"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := []syncFile{{
		Path: "07_wrapping_up.md",
		Content: `![Fruit market](fruit_market.png)
![Remote](https://example.com/remote.png)
![Static](/static/img/courses/intro-proto/static.png)
![Diagram](diagrams/field.png)`,
		ContentHash: "unused",
	}}

	assets, err := discoverCourseAssets(dir, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected two relative assets, got %#v", assets)
	}
	if assets[0].Path != "diagrams/field.png" || assets[1].Path != "fruit_market.png" {
		t.Fatalf("unexpected asset paths: %#v", assets)
	}
}

func TestDiscoverCourseAssetsErrorsWhenReferencedFileIsMissing(t *testing.T) {
	_, err := discoverCourseAssets(t.TempDir(), []syncFile{{
		Path:        "07_wrapping_up.md",
		Content:     `![Fruit market](fruit_market.png)`,
		ContentHash: "unused",
	}})
	if err == nil {
		t.Fatal("expected missing referenced image to fail")
	}
}

func TestAssetObjectKey(t *testing.T) {
	cfg := config{
		course:      "intro-proto",
		assetPrefix: "courses",
	}
	if got := cfg.assetObjectKey("diagrams/field.png"); got != "courses/intro-proto/diagrams/field.png" {
		t.Fatalf("object key = %q", got)
	}
}

func TestAssetManifestPathIsInsideCourseDir(t *testing.T) {
	cfg := config{dir: t.TempDir()}
	if got := cfg.assetManifestPath(); got != filepath.Join(cfg.dir, ".course-sync-assets.json") {
		t.Fatalf("manifest path = %q", got)
	}
}

func TestObjectKeyInCoursePrefix(t *testing.T) {
	cfg := config{
		course:      "intro-proto",
		assetPrefix: "courses",
	}
	if !cfg.objectKeyInCoursePrefix("courses/intro-proto/fruit_market.png") {
		t.Fatal("expected object under course prefix to be allowed")
	}
	if cfg.objectKeyInCoursePrefix("courses/other-course/fruit_market.png") {
		t.Fatal("expected object outside course prefix to be rejected")
	}
}
