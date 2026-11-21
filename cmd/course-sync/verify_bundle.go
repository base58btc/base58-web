package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kodylow/base58-website/internal/courseenv"
)

// Verify both the runtime fingerprint and the actual archive bytes, including
// after transferring a bundle from a CI builder to an author's machine.
func runVerifyBundle(args []string) error {
	fs := flag.NewFlagSet("verify-bundle", flag.ContinueOnError)
	dir := fs.String("dir", "", "course directory containing the prepared runtime")
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
	m, err := courseenv.ForFiles(courseSourceMap(files))
	if err != nil {
		return err
	}
	var total int64
	for _, archive := range m.Archives {
		file, err := os.Open(filepath.Join(*dir, filepath.FromSlash(archive.Path)))
		if err != nil {
			return err
		}
		digest := sha256.New()
		size, readErr := io.Copy(digest, file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if size != archive.Size || hex.EncodeToString(digest.Sum(nil)) != archive.SHA256 {
			return fmt.Errorf("course runtime archive size or SHA-256 mismatch: %s", archive.Path)
		}
		total += size
	}
	fmt.Printf("verified %s browser environment: %d archives, %.1f MiB compressed, runtime %s, content %s\n", m.System, len(m.Archives), float64(total)/(1024*1024), m.ArchiveHash(), courseenv.ContentHash(courseSourceMap(files)))
	return nil
}
