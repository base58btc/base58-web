package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func assetArchive(t *testing.T, omit string) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	archive := tar.NewWriter(gz)
	names := []string{"package/unused.txt"}
	for name := range assetFiles {
		if name != omit {
			names = append(names, name)
		}
	}
	for _, name := range names {
		content := "fixture:" + name
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestInstallAssetsAndRepairIncompleteCache(t *testing.T) {
	data := assetArchive(t, "")
	checksum := fmt.Sprintf("%x", sha256.Sum256(data))
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Write(data)
	}))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "assets")
	for range 2 {
		if err := install(server.Client(), server.URL, checksum, dest); err != nil {
			t.Fatal(err)
		}
	}
	if downloads.Load() != 1 {
		t.Fatal("complete installation was downloaded again")
	}
	for source, name := range assetFiles {
		content, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || string(content) != "fixture:"+source {
			t.Fatalf("incorrect installed asset %s: %q, %v", name, content, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "unused.txt")); !os.IsNotExist(err) {
		t.Fatal("installed an asset outside the allowlist")
	}
	if err := os.Remove(filepath.Join(dest, "unkernel.wasm")); err != nil {
		t.Fatal(err)
	}
	if err := install(server.Client(), server.URL, checksum, dest); err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != 2 || !installed(dest, checksum) {
		t.Fatal("checksum marker concealed a missing asset")
	}
}

func TestRejectedArchivePreservesInstallation(t *testing.T) {
	for _, tc := range []struct {
		name, omit, message string
		badChecksum         bool
	}{
		{name: "checksum mismatch", badChecksum: true, message: "SHA-256"},
		{name: "incomplete archive", omit: "package/dist/unkernel.wasm", message: "missing unkernel.wasm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := assetArchive(t, tc.omit)
			checksum := fmt.Sprintf("%x", sha256.Sum256(data))
			if tc.badChecksum {
				checksum = strings.Repeat("0", 64)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write(data)
			}))
			defer server.Close()
			dest := t.TempDir()
			for _, name := range []string{"web.mjs", ".archive-sha256"} {
				if err := os.WriteFile(filepath.Join(dest, name), []byte("previous"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			err := install(server.Client(), server.URL, checksum, dest)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %s, got %v", tc.message, err)
			}
			for _, name := range []string{"web.mjs", ".archive-sha256"} {
				content, err := os.ReadFile(filepath.Join(dest, name))
				if err != nil || string(content) != "previous" {
					t.Fatalf("rejected archive changed %s: %q, %v", name, content, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dest, "_chunks", "core.mjs")); !os.IsNotExist(err) {
				t.Fatal("rejected archive partially installed assets")
			}
		})
	}
}
