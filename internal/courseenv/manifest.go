// Package courseenv describes immutable Nix environments executed in a browser.
package courseenv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

const ManifestFile = "course-runtime.json"
const System = "aarch64-linux"

type Archive struct {
	StorePath string `json:"storePath"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
}

type Manifest struct {
	Format     int       `json:"format"`
	System     string    `json:"system"`
	SourceHash string    `json:"sourceHash"`
	Shell      string    `json:"shell"`
	Setup      string    `json:"setup"`
	Archives   []Archive `json:"archives"`
}

var storePathPattern = regexp.MustCompile(`^/nix/store/[0-9a-z]{32}-[A-Za-z0-9+._?=-]+$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Hash every source file, including imported scripts; never include the generated manifest.
func SourceHash(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for name := range files {
		if name != ManifestFile {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	var manifest strings.Builder
	for _, name := range paths {
		fmt.Fprintf(&manifest, "%s:%s\n", name, Hash([]byte(files[name])))
	}
	return Hash([]byte(manifest.String()))
}

func Parse(content string) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return nil, err
	}
	if m.Format != 1 || m.System != System || !hashPattern.MatchString(m.SourceHash) {
		return nil, errors.New("unsupported course runtime manifest")
	}
	if !strings.HasSuffix(m.Shell, "/bin/bash") || !storePathPattern.MatchString(strings.TrimSuffix(m.Shell, "/bin/bash")) {
		return nil, errors.New("course runtime must name its Nix bash executable")
	}
	if len(m.Archives) == 0 || len(m.Archives) > 4096 {
		return nil, errors.New("invalid course runtime archive count")
	}
	seen := map[string]bool{}
	hasShell := false
	for _, archive := range m.Archives {
		want := "runtime/" + m.SourceHash + "/" + path.Base(archive.StorePath) + ".nar.gz"
		if !storePathPattern.MatchString(archive.StorePath) || archive.Path != want || !hashPattern.MatchString(archive.SHA256) || archive.Size <= 0 || seen[archive.StorePath] {
			return nil, errors.New("invalid course runtime archive")
		}
		seen[archive.StorePath] = true
		hasShell = hasShell || m.Shell == archive.StorePath+"/bin/bash"
	}
	if !hasShell {
		return nil, errors.New("course runtime is missing bash from its closure")
	}
	return &m, nil
}

func ForFiles(files map[string]string) (*Manifest, error) {
	if _, ok := files["flake.nix"]; !ok {
		return nil, errors.New("course runtime requires flake.nix and flake.lock")
	}
	if _, ok := files["flake.lock"]; !ok {
		return nil, errors.New("course runtime requires flake.nix and flake.lock")
	}
	content, ok := files[ManifestFile]
	if !ok {
		return nil, errors.New("run course-sync bundle before publishing this course environment")
	}
	m, err := Parse(content)
	if err != nil {
		return nil, err
	}
	if m.SourceHash != SourceHash(files) {
		return nil, errors.New("course environment is stale; run course-sync bundle again")
	}
	return m, nil
}
