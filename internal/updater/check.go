package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckVersion queries the GitHub Releases API to check if a new version is available.
func (u *Updater) CheckVersion(ctx context.Context) (*VersionInfo, error) {
	// If current version is "dev", do not check/update
	if u.currentVersion == "dev" {
		return &VersionInfo{
			CurrentVersion: u.currentVersion,
			IsNewer:        false,
		}, nil
	}

	url := u.releasesURL
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "motoko-updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %s", resp.Status)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode github releases: %w", err)
	}

	var latestRelease *githubRelease
	var latestTag string

	for _, rel := range releases {
		// Filter by channel. Stable channel ignores prereleases.
		if u.channel == "stable" && rel.Prerelease {
			continue
		}

		// Check if release tag is newer than current version
		if CompareVersions(u.currentVersion, rel.TagName) < 0 {
			// Select the highest semantic version version found so far
			if latestTag == "" || CompareVersions(latestTag, rel.TagName) < 0 {
				latestTag = rel.TagName
				r := rel // copy for pointer safety
				latestRelease = &r
			}
		}
	}

	if latestRelease != nil {
		var downloadURL string
		expectedAsset := fmt.Sprintf("motoko_%s_%s.tar.gz", u.goos, u.goarch)
		for _, asset := range latestRelease.Assets {
			if asset.Name == expectedAsset {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}

		if downloadURL != "" {
			return &VersionInfo{
				CurrentVersion: u.currentVersion,
				NewVersion:     latestRelease.TagName,
				ReleaseURL:     latestRelease.HTMLURL,
				DownloadURL:    downloadURL,
				IsNewer:        true,
				Prerelease:     latestRelease.Prerelease,
			}, nil
		}
	}

	return &VersionInfo{
		CurrentVersion: u.currentVersion,
		IsNewer:        false,
	}, nil
}
