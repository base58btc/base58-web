package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type syncRequest struct {
	Files []syncFile `json:"files"`
}

type syncFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	ContentHash string `json:"sha256"`
}

type syncResponse struct {
	CourseSlug    string `json:"courseSlug"`
	VersionID     int64  `json:"versionId"`
	VersionNumber int    `json:"versionNumber"`
	Status        string `json:"status"`
	ContentHash   string `json:"contentHash"`
	FileCount     int    `json:"fileCount"`
}

type publishRequest struct {
	VersionID int64 `json:"versionId,omitempty"`
}

type syncResult struct {
	Response    syncResponse
	AssetCount  int
	DeleteCount int
}

type courseAsset struct {
	Path        string
	FullPath    string
	ContentHash string
}

type assetManifest struct {
	Objects map[string]assetManifestObject `json:"objects"`
}

type assetManifestObject struct {
	ObjectKey   string `json:"objectKey"`
	ContentHash string `json:"sha256"`
}

var markdownImagePattern = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)(?:\s+["'][^)]*["'])?\)`)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "sync":
		err = runSync(os.Args[2:])
	case "watch":
		err = runWatch(os.Args[2:])
	case "publish":
		err = runPublish(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "course-sync:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  course-sync sync  --server http://localhost:8080 --course intro-proto --dir courses/intro-proto
  course-sync watch --server http://localhost:8080 --course intro-proto --dir courses/intro-proto
  course-sync publish --server http://localhost:8080 --course intro-proto

Authentication:
  Pass --token or set BASE58_EDITOR_TOKEN.

Course assets:
  Relative markdown images are uploaded when COURSE_ASSET_BUCKET, SPACES_KEY,
  and SPACES_SECRET are set. Optional: COURSE_ASSET_REGION, COURSE_ASSET_PREFIX.`)
}

func runSync(args []string) error {
	cfg, _, err := parseConfig("sync", args, true)
	if err != nil {
		return err
	}
	result, err := syncCourse(cfg)
	if err != nil {
		return err
	}
	resp := result.Response
	fmt.Printf("synced %s: draft version %d (%d files, %s)\n", resp.CourseSlug, resp.VersionNumber, resp.FileCount, resp.ContentHash)
	if result.AssetCount > 0 {
		fmt.Printf("uploaded %d course asset(s)\n", result.AssetCount)
	}
	if result.DeleteCount > 0 {
		fmt.Printf("deleted %d stale course asset(s)\n", result.DeleteCount)
	}
	return nil
}

func runWatch(args []string) error {
	cfg, _, err := parseConfig("watch", args, true)
	if err != nil {
		return err
	}
	interval := time.Second
	lastHash := ""
	fmt.Printf("watching %s; syncing to %s\n", cfg.dir, cfg.server)
	for {
		hash, err := directoryHash(cfg.dir)
		if err != nil {
			return err
		}
		if hash != lastHash {
			result, err := syncCourse(cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "sync failed:", err)
			} else {
				resp := result.Response
				fmt.Printf("%s synced %s: draft version %d (%d files)\n", time.Now().Format("15:04:05"), resp.CourseSlug, resp.VersionNumber, resp.FileCount)
				if result.AssetCount > 0 {
					fmt.Printf("%s uploaded %d course asset(s)\n", time.Now().Format("15:04:05"), result.AssetCount)
				}
				if result.DeleteCount > 0 {
					fmt.Printf("%s deleted %d stale course asset(s)\n", time.Now().Format("15:04:05"), result.DeleteCount)
				}
				lastHash = hash
			}
		}
		time.Sleep(interval)
	}
}

func runPublish(args []string) error {
	cfg, versionID, err := parseConfig("publish", args, false)
	if err != nil {
		return err
	}
	resp, err := publishCourse(cfg, versionID)
	if err != nil {
		return err
	}
	fmt.Printf("published %s: version %d (%d files, %s)\n", resp.CourseSlug, resp.VersionNumber, resp.FileCount, resp.ContentHash)
	return nil
}

type config struct {
	server        string
	course        string
	dir           string
	token         string
	assetBucket   string
	assetRegion   string
	assetPrefix   string
	assetKey      string
	assetSecret   string
	assetEndpoint string
	noAssets      bool
}

func parseConfig(name string, args []string, requireDir bool) (config, int64, error) {
	var cfg config
	var versionID int64
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&cfg.server, "server", "http://localhost:8080", "base58 server URL")
	fs.StringVar(&cfg.course, "course", "", "course slug")
	fs.StringVar(&cfg.dir, "dir", "", "course markdown directory")
	fs.StringVar(&cfg.token, "token", os.Getenv("BASE58_EDITOR_TOKEN"), "editor API token")
	fs.StringVar(&cfg.assetBucket, "asset-bucket", os.Getenv("COURSE_ASSET_BUCKET"), "DigitalOcean Spaces bucket for referenced course assets")
	fs.StringVar(&cfg.assetRegion, "asset-region", getenvDefault("COURSE_ASSET_REGION", "nyc3"), "DigitalOcean Spaces region for course assets")
	fs.StringVar(&cfg.assetPrefix, "asset-prefix", getenvDefault("COURSE_ASSET_PREFIX", "courses"), "object key prefix for course assets")
	fs.StringVar(&cfg.assetEndpoint, "asset-endpoint", os.Getenv("COURSE_ASSET_ENDPOINT"), "DigitalOcean Spaces endpoint host, for example nyc3.digitaloceanspaces.com")
	fs.StringVar(&cfg.assetKey, "asset-key", firstNonEmpty(os.Getenv("SPACES_KEY"), os.Getenv("AWS_ACCESS_KEY_ID")), "DigitalOcean Spaces access key")
	fs.StringVar(&cfg.assetSecret, "asset-secret", firstNonEmpty(os.Getenv("SPACES_SECRET"), os.Getenv("AWS_SECRET_ACCESS_KEY")), "DigitalOcean Spaces secret key")
	fs.BoolVar(&cfg.noAssets, "no-assets", false, "skip uploading referenced markdown images")
	fs.Int64Var(&versionID, "version-id", 0, "draft course version id to publish")
	if err := fs.Parse(args); err != nil {
		return cfg, 0, err
	}
	if requireDir && cfg.dir == "" {
		return cfg, 0, fmt.Errorf("--dir is required")
	}
	if cfg.course == "" && cfg.dir != "" {
		cfg.course = filepath.Base(filepath.Clean(cfg.dir))
	}
	if cfg.course == "" {
		return cfg, 0, fmt.Errorf("--course is required")
	}
	if cfg.token == "" {
		return cfg, 0, fmt.Errorf("--token or BASE58_EDITOR_TOKEN is required")
	}
	cfg.server = strings.TrimRight(cfg.server, "/")
	cfg.assetPrefix = strings.Trim(strings.TrimSpace(cfg.assetPrefix), "/")
	cfg.assetEndpoint = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(cfg.assetEndpoint, "https://"), "http://"))
	return cfg, versionID, nil
}

func syncCourse(cfg config) (syncResult, error) {
	files, err := readCourseFiles(cfg.dir)
	if err != nil {
		return syncResult{}, err
	}
	assets, err := discoverCourseAssets(cfg.dir, files)
	if err != nil {
		return syncResult{}, err
	}
	uploadedAssets := 0
	deletedAssets := 0
	if cfg.assetsEnabled() {
		uploadedAssets, err = uploadCourseAssets(cfg, assets)
		if err != nil {
			return syncResult{}, err
		}
		deletedAssets, err = reconcileCourseAssetManifest(cfg, assets)
		if err != nil {
			return syncResult{}, err
		}
	}
	payload, err := json.Marshal(syncRequest{Files: files})
	if err != nil {
		return syncResult{}, err
	}
	syncURL := cfg.server + "/api/editor/courses/" + url.PathEscape(cfg.course) + "/sync"
	req, err := http.NewRequest(http.MethodPost, syncURL, bytes.NewReader(payload))
	if err != nil {
		return syncResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return syncResult{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return syncResult{}, fmt.Errorf("server returned %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var out syncResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return syncResult{}, err
	}
	return syncResult{Response: out, AssetCount: uploadedAssets, DeleteCount: deletedAssets}, nil
}

func publishCourse(cfg config, versionID int64) (syncResponse, error) {
	payload, err := json.Marshal(publishRequest{VersionID: versionID})
	if err != nil {
		return syncResponse{}, err
	}
	publishURL := cfg.server + "/api/editor/courses/" + url.PathEscape(cfg.course) + "/publish"
	req, err := http.NewRequest(http.MethodPost, publishURL, bytes.NewReader(payload))
	if err != nil {
		return syncResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return syncResponse{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return syncResponse{}, fmt.Errorf("server returned %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var out syncResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return syncResponse{}, err
	}
	return out, nil
}

func readCourseFiles(dir string) ([]syncFile, error) {
	var files []syncFile
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files = append(files, syncFile{
			Path:        rel,
			Content:     string(content),
			ContentHash: hashBytes(content),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func directoryHash(dir string) (string, error) {
	files, err := readCourseFiles(dir)
	if err != nil {
		return "", err
	}
	assets, err := discoverCourseAssets(dir, files)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, file := range files {
		b.WriteString(file.Path)
		b.WriteString(":")
		b.WriteString(file.ContentHash)
		b.WriteString("\n")
	}
	for _, asset := range assets {
		b.WriteString(asset.Path)
		b.WriteString(":")
		b.WriteString(asset.ContentHash)
		b.WriteString("\n")
	}
	return hashBytes([]byte(b.String())), nil
}

func discoverCourseAssets(dir string, files []syncFile) ([]courseAsset, error) {
	seen := map[string]bool{}
	var assets []courseAsset
	for _, file := range files {
		for _, raw := range referencedMarkdownImages(file.Content) {
			assetPath, ok := cleanRelativeAssetPath(raw)
			if !ok || seen[assetPath] {
				continue
			}
			seen[assetPath] = true
			fullPath := filepath.Join(dir, filepath.FromSlash(assetPath))
			info, err := os.Stat(fullPath)
			if err != nil {
				return nil, fmt.Errorf("referenced image %q was not found at %s", raw, fullPath)
			}
			if info.IsDir() {
				return nil, fmt.Errorf("referenced image %q points to a directory", raw)
			}
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, err
			}
			assets = append(assets, courseAsset{
				Path:        assetPath,
				FullPath:    fullPath,
				ContentHash: hashBytes(content),
			})
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, nil
}

func referencedMarkdownImages(content string) []string {
	matches := markdownImagePattern.FindAllStringSubmatch(content, -1)
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			refs = append(refs, strings.TrimSpace(match[1]))
		}
	}
	return refs
}

func cleanRelativeAssetPath(raw string) (string, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), "<>")
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "#") {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.IsAbs() {
		return "", false
	}
	if err == nil {
		raw = parsed.Path
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.ToSlash(raw)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "\x00") {
		return "", false
	}
	return clean, true
}

func uploadCourseAssets(cfg config, assets []courseAsset) (int, error) {
	uploaded := 0
	for _, asset := range assets {
		if err := uploadCourseAsset(cfg, asset); err != nil {
			return uploaded, err
		}
		uploaded++
	}
	return uploaded, nil
}

func uploadCourseAsset(cfg config, asset courseAsset) error {
	body, err := os.ReadFile(asset.FullPath)
	if err != nil {
		return err
	}
	objectKey := cfg.assetObjectKey(asset.Path)
	endpointHost := cfg.assetEndpointHost()
	assetURL := "https://" + cfg.assetBucket + "." + endpointHost + "/" + pathEscapeAssetKey(objectKey)
	req, err := http.NewRequest(http.MethodPut, assetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	contentType := contentTypeForAsset(asset.FullPath)
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", date)
	req.Header.Set("x-amz-acl", "public-read")
	req.Header.Set("Authorization", spacesAuthorization(cfg, objectKey, contentType, date))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("upload %s failed with %s: %s", asset.Path, res.Status, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func reconcileCourseAssetManifest(cfg config, assets []courseAsset) (int, error) {
	manifest, err := loadAssetManifest(cfg)
	if err != nil {
		return 0, err
	}
	current := map[string]assetManifestObject{}
	for _, asset := range assets {
		current[asset.Path] = assetManifestObject{
			ObjectKey:   cfg.assetObjectKey(asset.Path),
			ContentHash: asset.ContentHash,
		}
	}

	deleted := 0
	for assetPath, previous := range manifest.Objects {
		if _, ok := current[assetPath]; ok {
			continue
		}
		if previous.ObjectKey == "" {
			continue
		}
		if !cfg.objectKeyInCoursePrefix(previous.ObjectKey) {
			return deleted, fmt.Errorf("refusing to delete asset outside course prefix: %s", previous.ObjectKey)
		}
		if err := deleteCourseAssetObject(cfg, previous.ObjectKey); err != nil {
			return deleted, err
		}
		deleted++
	}

	manifest.Objects = current
	if err := saveAssetManifest(cfg, manifest); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func loadAssetManifest(cfg config) (assetManifest, error) {
	manifest := assetManifest{Objects: map[string]assetManifestObject{}}
	content, err := os.ReadFile(cfg.assetManifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return manifest, nil
		}
		return manifest, err
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return manifest, nil
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Objects == nil {
		manifest.Objects = map[string]assetManifestObject{}
	}
	return manifest, nil
}

func saveAssetManifest(cfg config, manifest assetManifest) error {
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(cfg.assetManifestPath(), content, 0o600)
}

func deleteCourseAssetObject(cfg config, objectKey string) error {
	endpointHost := cfg.assetEndpointHost()
	assetURL := "https://" + cfg.assetBucket + "." + endpointHost + "/" + pathEscapeAssetKey(objectKey)
	req, err := http.NewRequest(http.MethodDelete, assetURL, nil)
	if err != nil {
		return err
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", spacesDeleteAuthorization(cfg, objectKey, date))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("delete %s failed with %s: %s", objectKey, res.Status, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (cfg config) assetsEnabled() bool {
	return !cfg.noAssets && cfg.assetBucket != "" && cfg.assetKey != "" && cfg.assetSecret != ""
}

func (cfg config) assetEndpointHost() string {
	if cfg.assetEndpoint != "" {
		return cfg.assetEndpoint
	}
	return cfg.assetRegion + ".digitaloceanspaces.com"
}

func (cfg config) assetObjectKey(assetPath string) string {
	parts := []string{}
	if cfg.assetPrefix != "" {
		parts = append(parts, cfg.assetPrefix)
	}
	parts = append(parts, cfg.course, assetPath)
	return strings.Join(parts, "/")
}

func (cfg config) assetManifestPath() string {
	return filepath.Join(cfg.dir, ".course-sync-assets.json")
}

func (cfg config) courseObjectPrefix() string {
	parts := []string{}
	if cfg.assetPrefix != "" {
		parts = append(parts, cfg.assetPrefix)
	}
	parts = append(parts, cfg.course)
	return strings.Join(parts, "/") + "/"
}

func (cfg config) objectKeyInCoursePrefix(objectKey string) bool {
	return strings.HasPrefix(objectKey, cfg.courseObjectPrefix())
}

func spacesAuthorization(cfg config, objectKey, contentType, date string) string {
	stringToSign := strings.Join([]string{
		http.MethodPut,
		"",
		contentType,
		date,
		"x-amz-acl:public-read",
		"/" + cfg.assetBucket + "/" + objectKey,
	}, "\n")
	mac := hmac.New(sha1.New, []byte(cfg.assetSecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return "AWS " + cfg.assetKey + ":" + signature
}

func spacesDeleteAuthorization(cfg config, objectKey, date string) string {
	stringToSign := strings.Join([]string{
		http.MethodDelete,
		"",
		"",
		date,
		"/" + cfg.assetBucket + "/" + objectKey,
	}, "\n")
	mac := hmac.New(sha1.New, []byte(cfg.assetSecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return "AWS " + cfg.assetKey + ":" + signature
}

func contentTypeForAsset(path string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func pathEscapeAssetKey(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
