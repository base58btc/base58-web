package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kodylow/base58-website/internal/courseenv"
)

type nixVariable struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}
type nixEnvironment struct {
	Variables map[string]nixVariable `json:"variables"`
	Functions map[string]string      `json:"bashFunctions"`
}

var shellIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func runBundle(args []string) error {
	fs := flag.NewFlagSet("bundle", flag.ContinueOnError)
	dir := fs.String("dir", "", "course directory containing flake.nix and flake.lock")
	nix := fs.String("nix", "nix", "Nix executable")
	nixStore := fs.String("nix-store", "nix-store", "nix-store executable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("--dir is required")
	}
	files, err := readCourseFiles(*dir)
	if err != nil {
		return err
	}
	source := courseSourceMap(files)
	if _, ok := source["flake.lock"]; !ok {
		return fmt.Errorf("course bundle requires flake.nix and flake.lock")
	}
	// Work from the exact uploaded source set. Ignored files and previous bundles
	// cannot silently influence evaluation, and Nix cannot rewrite the author's lock.
	root, err := os.MkdirTemp("", "base58-course-flake-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	for name, content := range source {
		if name == courseenv.ManifestFile {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte(content), 0o600); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	profile := filepath.Join(root, ".base58-environment")
	cmd := exec.CommandContext(ctx, *nix, "print-dev-env", "--json", "--profile", profile, "--no-update-lock-file", "--no-write-lock-file", "path:"+root+"#devShells."+courseenv.System+".default")
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("preparing %s shell (requires a Linux builder or substitutes): %w", courseenv.System, err)
	}
	var env nixEnvironment
	if err := json.Unmarshal(output, &env); err != nil {
		return err
	}
	setup, shell, err := renderNixEnvironment(env)
	if err != nil {
		return err
	}
	// Ask Nix for its recorded references. Scanning environment text picks up
	// fake /nix/store/eeee… paths from compiler prefix maps and misses context.
	profilePath, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return fmt.Errorf("reading Nix environment profile: %w", err)
	}
	cmd = exec.CommandContext(ctx, *nix, "path-info", "--recursive", "--json", profilePath)
	cmd.Stderr = os.Stderr
	closure, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("reading shell closure: %w", err)
	}
	var info map[string]json.RawMessage
	if err := json.Unmarshal(closure, &info); err != nil {
		return err
	}
	pathsList := make([]string, 0, len(info))
	for storePath := range info {
		// The profile itself contains the unsanitized environment JSON. Ship
		// its dependencies and the sanitized setup script, never that JSON.
		if storePath == profilePath {
			continue
		}
		pathsList = append(pathsList, storePath)
	}
	sort.Strings(pathsList)
	m := courseenv.Manifest{Format: 1, System: courseenv.System, SourceHash: courseenv.SourceHash(source), Shell: shell, Setup: setup}
	for _, storePath := range pathsList {
		asset := "runtime/" + m.SourceHash + "/" + filepath.Base(storePath) + ".nar.gz"
		destination := filepath.Join(*dir, filepath.FromSlash(asset))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		digest, size, err := exportNAR(ctx, *nixStore, storePath, destination)
		if err != nil {
			return err
		}
		m.Archives = append(m.Archives, courseenv.Archive{StorePath: storePath, Path: asset, SHA256: digest, Size: size})
	}
	content, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if _, err := courseenv.Parse(string(content)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*dir, courseenv.ManifestFile), append(content, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("prepared %s browser environment: %d store paths; run course-sync sync to upload\n", m.System, len(m.Archives))
	return nil
}

func courseSourceMap(files []syncFile) map[string]string {
	source := make(map[string]string, len(files))
	for _, file := range files {
		source[file.Path] = file.Content
	}
	return source
}

func quoteShell(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func renderNixEnvironment(env nixEnvironment) (string, string, error) {
	var names []string
	for name := range env.Variables {
		names = append(names, name)
	}
	sort.Strings(names)
	var setup strings.Builder
	var shell string
	for _, name := range names {
		if !shellIdentifier.MatchString(name) || privateEnvironmentName(name) {
			continue
		}
		v := env.Variables[name]
		var value string
		if v.Type == "exported" || v.Type == "var" {
			if err := json.Unmarshal(v.Value, &value); err != nil {
				return "", "", err
			}
			prefix := ""
			if v.Type == "exported" {
				prefix = "export "
			}
			fmt.Fprintf(&setup, "%s%s=%s\n", prefix, name, quoteShell(value))
			if name == "SHELL" {
				shell = value
			}
		} else if v.Type == "array" {
			var values []string
			if err := json.Unmarshal(v.Value, &values); err != nil {
				return "", "", err
			}
			fmt.Fprintf(&setup, "%s=(", name)
			for _, value := range values {
				fmt.Fprintf(&setup, " %s", quoteShell(value))
			}
			setup.WriteString(" )\n")
		} else {
			return "", "", fmt.Errorf("unsupported Nix variable type %q for %s", v.Type, name)
		}
	}
	names = names[:0]
	for name := range env.Functions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !shellIdentifier.MatchString(name) {
			return "", "", fmt.Errorf("unsupported shell function name %q", name)
		}
		fmt.Fprintf(&setup, "%s() {\n%s\n}\n", name, env.Functions[name])
	}
	if shell == "" {
		return "", "", fmt.Errorf("Nix environment is missing SHELL")
	}
	setup.WriteString("export HOME=/data/workspace TMPDIR=/tmp\ncd /data/workspace\n")
	return setup.String(), shell, nil
}

func privateEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "HOME", "PWD", "OLDPWD", "TMPDIR", "TMP", "TEMP", "SHLVL", "DATABASE_URL", "DB_DRIVER", "BASHOPTS", "SHELLOPTS":
		return true
	}
	for _, suffix := range []string{"_SECRET", "_TOKEN", "_PASSWORD", "_CREDENTIAL", "_CREDENTIALS", "_PRIVATE_KEY"} {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

func exportNAR(ctx context.Context, nixStore, storePath, destination string) (string, int64, error) {
	file, err := os.CreateTemp(filepath.Dir(destination), ".nar-")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	zip := gzip.NewWriter(io.MultiWriter(file, hash))
	cmd := exec.CommandContext(ctx, nixStore, "--dump", storePath)
	cmd.Stdout = zip
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("exporting %s: %w", storePath, err)
	}
	if err := zip.Close(); err != nil {
		return "", 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(file.Name(), destination); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), info.Size(), nil
}
