package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed qemu-assets.json
var qemuPins []byte

// The upstream release omits these boot files. Keep the hash-verified files
// with their build sources referenced in static/licenses/qemu/SOURCES.txt,
// rather than depending on a mutable trynix.dev deployment at website build.
//
//go:embed guest/*
var qemuGuest embed.FS

type qemuAsset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Hash   string `json:"hash"`
	Source string `json:"source,omitempty"`
}

func installQEMU(client *http.Client, dest string) error {
	var assets []qemuAsset
	if err := json.Unmarshal(qemuPins, &assets); err != nil {
		return err
	}
	for _, asset := range assets {
		want, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(asset.Hash, "sha256-"))
		if err != nil || len(want) != sha256.Size {
			return fmt.Errorf("invalid QEMU checksum for %s", asset.Name)
		}
		target := filepath.Join(dest, asset.Name)
		if data, err := os.ReadFile(target); err == nil {
			hash := sha256.Sum256(data)
			if bytes.Equal(hash[:], want) {
				continue
			}
		}
		data, err := qemuAssetBytes(client, asset)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		if !bytes.Equal(hash[:], want) {
			return fmt.Errorf("QEMU asset %s failed its SHA-256 check", asset.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(target), ".qemu-")
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr == nil {
			writeErr = os.Chmod(file.Name(), 0644)
		}
		if writeErr == nil {
			writeErr = os.Rename(file.Name(), target)
		}
		os.Remove(file.Name())
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func qemuAssetBytes(client *http.Client, asset qemuAsset) ([]byte, error) {
	if asset.Source != "" {
		return qemuGuest.ReadFile(asset.Source)
	}
	resp, err := client.Get(asset.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", asset.Name, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
}
