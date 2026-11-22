package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestEmbeddedQEMUGuestMatchesUpstreamPins(t *testing.T) {
	var assets []qemuAsset
	if err := json.Unmarshal(qemuPins, &assets); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, asset := range assets {
		if asset.Source == "" {
			continue
		}
		data, err := qemuAssetBytes(nil, asset)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		if "sha256-"+base64.StdEncoding.EncodeToString(hash[:]) != asset.Hash {
			t.Fatalf("guest image %s changed", asset.Name)
		}
		checked++
	}
	if checked != 2 {
		t.Fatal("expected the kernel and initramfs")
	}
}
