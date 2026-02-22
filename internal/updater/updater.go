// Package updater provides self-update functionality via GitHub Releases.
package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	repo       = "yhzion/claude-postman"
	apiURL     = "https://api.github.com/repos/" + repo + "/releases/latest"
	binaryName = "claude-postman"
)

// Release holds GitHub release metadata.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset holds a release asset's download URL and name.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// HTTPClient abstracts HTTP for testability.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// Updater checks and performs updates.
type Updater struct {
	CurrentVersion string
	Client         HTTPClient
}

// New creates an Updater with the default HTTP client.
func New(currentVersion string) *Updater {
	return &Updater{
		CurrentVersion: currentVersion,
		Client:         &http.Client{Timeout: 10 * time.Second},
	}
}

// CheckLatest fetches the latest release tag from GitHub.
// Returns the release info, or nil if already up to date.
func (u *Updater) CheckLatest() (*Release, error) {
	resp, err := u.Client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &rel, nil
}

// IsNewer returns true if the release is newer than current version.
func (u *Updater) IsNewer(rel *Release) bool {
	if u.CurrentVersion == "dev" {
		return false // dev builds don't auto-update
	}
	return rel.TagName != u.CurrentVersion
}

// AssetName returns the expected binary name for the current platform.
func AssetName() string {
	return fmt.Sprintf("%s-%s-%s", binaryName, runtime.GOOS, runtime.GOARCH)
}

// FindAsset finds the download URL for the current platform.
func FindAsset(rel *Release) (string, error) {
	name := AssetName()
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no asset found for %s", name)
}

// Download fetches a URL and writes it to a temporary file.
// Returns the temp file path.
func (u *Updater) Download(url string) (string, error) {
	resp, err := u.Client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "claude-postman-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	return tmp.Name(), nil
}

// Replace swaps the current binary with the downloaded one.
func Replace(newBinary string) error {
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current binary: %w", err)
	}

	// Rename current → current.old (backup)
	backup := current + ".old"
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	// Move new → current
	if err := os.Rename(newBinary, current); err != nil {
		// Restore backup on failure
		_ = os.Rename(backup, current)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Remove backup
	_ = os.Remove(backup)
	return nil
}

// RunUpdate performs the full self-update flow.
func (u *Updater) RunUpdate() error {
	fmt.Println("Checking for updates...")

	rel, err := u.CheckLatest()
	if err != nil {
		return err
	}

	if !u.IsNewer(rel) {
		fmt.Printf("Already up to date (%s)\n", u.CurrentVersion)
		return nil
	}

	fmt.Printf("New version available: %s → %s\n", u.CurrentVersion, rel.TagName)

	url, err := FindAsset(rel)
	if err != nil {
		return err
	}

	fmt.Println("Downloading...")
	tmpPath, err := u.Download(url)
	if err != nil {
		return err
	}

	fmt.Println("Installing...")
	if err := Replace(tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	fmt.Printf("Updated to %s\n", rel.TagName)
	return nil
}

// CheckAndNotify checks for updates and prints a hint if available.
// Non-blocking: errors are silently ignored.
func (u *Updater) CheckAndNotify() {
	if u.CurrentVersion == "dev" {
		return
	}

	rel, err := u.CheckLatest()
	if err != nil || !u.IsNewer(rel) {
		return
	}

	latest := rel.TagName
	current := u.CurrentVersion
	fmt.Fprintf(os.Stderr,
		"\n  New version available: %s → %s\n  Run 'claude-postman update' to upgrade.\n\n",
		current, latest,
	)
}

// NormalizeVersion strips 'v' prefix for comparison.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}
