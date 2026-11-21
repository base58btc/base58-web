package main

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kodylow/base58-website/internal/courseenv"
)

func TestCapturedShellPreservesQuotingAndHook(t *testing.T) {
	var env nixEnvironment
	if err := json.Unmarshal([]byte(`{"variables":{"SHELL":{"type":"exported","value":"/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bash/bin/bash"},"COURSE":{"type":"exported","value":"hello 'world'; $(printf unsafe)"},"shellHook":{"type":"var","value":"export FROM_HOOK=yes"},"DATABASE_URL":{"type":"exported","value":"secret"},"RUNNER_SECRET":{"type":"exported","value":"secret"}},"bashFunctions":{"greet":"printf '%s' hello"}}`), &env); err != nil {
		t.Fatal(err)
	}
	setup, _, err := renderNixEnvironment(env)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(setup, "secret") {
		t.Fatal("captured host credentials")
	}
	// Exercise bash's interpretation, including single quotes and command-like text.
	setup = strings.ReplaceAll(setup, "cd /data/workspace", "cd /tmp")
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", setup+"\neval \"$shellHook\"\nprintf '%s|%s|' \"$COURSE\" \"$FROM_HOOK\"\ngreet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash: %v: %s", err, output)
	}
	if string(output) != "hello 'world'; $(printf unsafe)|yes|hello" {
		t.Fatalf("incorrect environment: %s", output)
	}
}

func TestBundleExportsProfileDependenciesAndExcludesRawEnvironment(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	course := filepath.Join(dir, "course")
	if err := os.Mkdir(course, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(course, "01_intro.md"), "# Intro\n", 0o600)
	write(filepath.Join(course, "flake.nix"), "{}", 0o600)
	write(filepath.Join(course, "flake.lock"), "{}", 0o600)
	profile := filepath.Join(dir, "raw-environment")
	write(profile, "private environment metadata", 0o600)
	shellStore := "/nix/store/" + strings.Repeat("a", 32) + "-bash"
	env, _ := json.Marshal(nixEnvironment{Variables: map[string]nixVariable{
		"SHELL":              {Type: "exported", Value: json.RawMessage(`"` + shellStore + `/bin/bash"`)},
		"NIX_CFLAGS_COMPILE": {Type: "exported", Value: json.RawMessage(`"-fdebug-prefix-map=/nix/store/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee-fake=/source"`)},
	}})
	envFile := filepath.Join(dir, "env.json")
	write(envFile, string(env), 0o600)
	closure, _ := json.Marshal(map[string]any{profile: map[string]any{}, shellStore: map[string]any{}})
	nix := filepath.Join(dir, "nix")
	write(nix, "#!/bin/sh\nset -eu\ncase \"$1\" in\nprint-dev-env)\n shift; while [ $# -gt 0 ]; do if [ \"$1\" = --profile ]; then shift; ln -s "+quoteShell(profile)+" \"$1\"; fi; shift; done\ncat "+quoteShell(envFile)+"\n;;\npath-info)\n test \"$4\" = "+quoteShell(profile)+"\nprintf '%s' "+quoteShell(string(closure))+"\n;;\n*) exit 1;;\nesac\n", 0o700)
	nixStore := filepath.Join(dir, "nix-store")
	write(nixStore, "#!/bin/sh\nset -eu\ntest \"$1\" = --dump\ntest \"$2\" = "+quoteShell(shellStore)+"\nprintf 'archive bytes'\n", 0o700)
	if err := runBundle([]string{"--dir", course, "--nix", nix, "--nix-store", nixStore}); err != nil {
		t.Fatal(err)
	}
	files, err := readCourseFiles(course)
	if err != nil {
		t.Fatal(err)
	}
	m, err := courseenv.ForFiles(courseSourceMap(files))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Archives) != 1 || m.Archives[0].StorePath != shellStore {
		t.Fatalf("incorrect exported closure: %#v", m.Archives)
	}
	archive, err := os.Open(filepath.Join(course, m.Archives[0].Path))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	unzip, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer unzip.Close()
	contents, err := io.ReadAll(unzip)
	if err != nil || string(contents) != "archive bytes" {
		t.Fatalf("incorrect archive: %q, %v", contents, err)
	}
}
