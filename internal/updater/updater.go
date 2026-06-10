package updater

import (
	"strings"
)

// Config holds the configuration for the Updater.
type Config struct {
	CurrentVersion string
	GOOS           string
	GOARCH         string
}

// Updater is the main updater structure.
type Updater struct {
	currentVersion string
	channel        string // "stable" or "alpha"
	goos           string
	goarch         string
	releasesURL    string // Overrideable in tests
}

// VersionInfo holds version check results.
type VersionInfo struct {
	CurrentVersion string `json:"current_version"`
	NewVersion     string `json:"new_version"`
	ReleaseURL     string `json:"release_url"`
	DownloadURL    string `json:"download_url"`
	IsNewer        bool   `json:"is_newer"`
	Prerelease     bool   `json:"prerelease"`
}

// NewUpdater creates a new Updater instance.
func NewUpdater(cfg Config) *Updater {
	channel := "stable"
	// If current version contains a hyphen (e.g. "v0.1.8-alpha"), we track prereleases.
	if strings.Contains(cfg.CurrentVersion, "-") {
		channel = "alpha"
	}

	return &Updater{
		currentVersion: cfg.CurrentVersion,
		channel:        channel,
		goos:           cfg.GOOS,
		goarch:         cfg.GOARCH,
		releasesURL:    "https://api.github.com/repos/Hoosk/motoko/releases",
	}
}
