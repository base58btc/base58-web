// course-vm-assets installs the pinned browser VM assets for website builds.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	version     = "0.2.0-261119-040e142"
	archiveURL  = "https://registry.npmjs.org/unkernel/-/unkernel-" + version + ".tgz"
	archiveHash = "001642f481d94dfd441fc289b005939806e198b06d6ab2b27ff4e4e07d7ce161"
)

var assetFiles = map[string]string{
	"package/dist/web.mjs":          "web.mjs",
	"package/dist/_chunks/core.mjs": "_chunks/core.mjs",
	"package/dist/unkernel.wasm":    "unkernel.wasm",
	"package/LICENSE":               "LICENSE",
	"package/LICENSE-MIT":           "LICENSE-MIT",
}

func main() {
	dir := flag.String("dir", filepath.Join("static", "vendor", "unkernel-"+version), "asset destination directory")
	qemuDir := flag.String("qemu-dir", filepath.Join("static", "vendor", "qemu-20260913"), "QEMU asset destination directory")
	flag.Parse()
	if err := install(&http.Client{Timeout: 2 * time.Minute}, archiveURL, archiveHash, *dir); err != nil {
		log.Fatal(err)
	}
	if err := installQEMU(&http.Client{Timeout: 2 * time.Minute}, *qemuDir); err != nil {
		log.Fatal(err)
	}
}

func install(client *http.Client, url, checksum, dest string) error {
	if installed(dest, checksum) {
		return nil
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download VM assets: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != checksum {
		return fmt.Errorf("VM asset archive failed its SHA-256 check")
	}

	// Validate and unpack every required file before updating the installation.
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".course-vm-assets-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := unpack(data, stage); err != nil {
		return err
	}
	for _, name := range assetFiles {
		target := filepath.Join(dest, name)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(stage, name), target); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dest, ".archive-sha256"), []byte(checksum), 0644)
}

func installed(dest, checksum string) bool {
	marker, err := os.ReadFile(filepath.Join(dest, ".archive-sha256"))
	if err != nil || string(marker) != checksum {
		return false
	}
	for _, name := range assetFiles {
		info, err := os.Lstat(filepath.Join(dest, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func unpack(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	seen := make(map[string]bool)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, wanted := assetFiles[header.Name]
		if !wanted {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || seen[name] {
			return fmt.Errorf("invalid VM asset archive entry %q", header.Name)
		}
		// Archive paths never become output paths; only the fixed allowlist does.
		target := filepath.Join(dest, name)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, archive)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		seen[name] = true
	}
	for _, name := range assetFiles {
		if !seen[name] {
			return fmt.Errorf("VM asset archive is missing %s", name)
		}
	}
	return nil
}
