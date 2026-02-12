package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const repo = "montanaflynn/botctl"

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CommitInfo returns the short commit hash and dirty flag from Go build info.
func CommitInfo() (hash string, dirty bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				hash = s.Value[:7]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return
}

// CheckLatest returns the latest release version (without "v" prefix) from GitHub.
func CheckLatest() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github api: %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}

	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// IsNewer returns true if latest is a higher semver than current.
func IsNewer(current, latest string) bool {
	if current == "dev" || latest == "" {
		return false
	}
	cur := parseVersion(current)
	lat := parseVersion(latest)
	for i := 0; i < 3; i++ {
		if lat[i] > cur[i] {
			return true
		}
		if lat[i] < cur[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		p, _, _ = strings.Cut(p, "-")
		result[i], _ = strconv.Atoi(p)
	}
	return result
}

// SelfUpdate downloads the latest release binary and replaces the current executable.
// Returns the new version string on success.
func SelfUpdate() (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	resp, err := client.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return "", fmt.Errorf("check latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github api: %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("parse release: %w", err)
	}

	version := strings.TrimPrefix(rel.TagName, "v")

	// Find matching asset for this OS/arch
	assetName := fmt.Sprintf("botctl-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("no binary for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
	}

	// Download binary
	dlResp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		return "", fmt.Errorf("download: %s", dlResp.Status)
	}

	// Resolve current executable path (follow symlinks)
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}

	// Write to temp file in same directory (ensures same filesystem for atomic rename)
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".botctl-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, dlResp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write binary: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("chmod: %w", err)
	}

	// Replace binary (atomic rename on Unix, rename-and-swap on Windows)
	if err := replaceExecutable(tmpPath, exe); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("replace binary: %w", err)
	}

	return version, nil
}
