package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantPre   string
		wantMajor int
		wantMinor int
		wantPatch int
		wantErr   bool
	}{
		{name: "simple version", version: "1.2.3", wantMajor: 1, wantMinor: 2, wantPatch: 3, wantPre: "", wantErr: false},
		{name: "version with leading v", version: "v1.2.3", wantMajor: 1, wantMinor: 2, wantPatch: 3, wantPre: "", wantErr: false},
		{name: "version with leading V", version: "V1.2.3", wantMajor: 1, wantMinor: 2, wantPatch: 3, wantPre: "", wantErr: false},
		{name: "version with alpha pre-release", version: "v1.2.3-alpha", wantMajor: 1, wantMinor: 2, wantPatch: 3, wantPre: "alpha", wantErr: false},
		{name: "version with complex pre-release", version: "v0.1.8-alpha.2", wantMajor: 0, wantMinor: 1, wantPatch: 8, wantPre: "alpha.2", wantErr: false},
		{name: "only major and minor", version: "1.2", wantMajor: 1, wantMinor: 2, wantPatch: 0, wantPre: "", wantErr: false},
		{name: "only major", version: "1", wantMajor: 1, wantMinor: 0, wantPatch: 0, wantPre: "", wantErr: false},
		{name: "empty version", version: "", wantMajor: 0, wantMinor: 0, wantPatch: 0, wantPre: "", wantErr: true},
		{name: "non-numeric version", version: "dev", wantMajor: 0, wantMinor: 0, wantPatch: 0, wantPre: "", wantErr: true},
		{name: "invalid format", version: "abc.def.ghi", wantMajor: 0, wantMinor: 0, wantPatch: 0, wantPre: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maj, min, pat, pre, err := ParseVersion(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.version)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.version, err)
			}
			if maj != tt.wantMajor || min != tt.wantMinor || pat != tt.wantPatch || pre != tt.wantPre {
				t.Errorf("ParseVersion(%q) = (%d, %d, %d, %q); want (%d, %d, %d, %q)",
					tt.version, maj, min, pat, pre, tt.wantMajor, tt.wantMinor, tt.wantPatch, tt.wantPre)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int // -1 if current < latest, 0 if equal, 1 if current > latest
	}{
		{"equal basic", "v1.2.3", "v1.2.3", 0},
		{"equal basic no v", "1.2.3", "1.2.3", 0},
		{"latest is newer major", "v1.2.3", "v2.0.0", -1},
		{"latest is newer minor", "v1.2.3", "v1.3.0", -1},
		{"latest is newer patch", "v1.2.3", "v1.2.4", -1},
		{"current is newer major", "v2.2.3", "v1.9.9", 1},
		{"current is newer minor", "v1.3.3", "v1.2.9", 1},
		{"current is newer patch", "v1.2.10", "v1.2.9", 1},
		{"latest has no pre-release (newer)", "v1.2.3-alpha", "v1.2.3", -1},
		{"current has no pre-release (newer)", "v1.2.3", "v1.2.3-alpha", 1},
		{"both have pre-release (latest newer)", "v1.2.3-alpha", "v1.2.3-beta", -1},
		{"both have pre-release (current newer)", "v1.2.3-rc", "v1.2.3-beta", 1},
		{"dev is not comparable (equal)", "dev", "v1.2.3", 0},
		{"invalid is not comparable (equal)", "v1.2.3", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d; want %d", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestNewUpdaterChannel(t *testing.T) {
	tests := []struct {
		version string
		channel string
	}{
		{"v1.2.3", "stable"},
		{"1.0.0", "stable"},
		{"v0.1.8-alpha", "alpha"},
		{"v1.2.0-beta.1", "alpha"},
		{"dev", "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			upd := NewUpdater(Config{
				CurrentVersion: tt.version,
				GOOS:           "linux",
				GOARCH:         "amd64",
			})
			if upd.channel != tt.channel {
				t.Errorf("NewUpdater(%q) channel = %q; want %q", tt.version, upd.channel, tt.channel)
			}
		})
	}
}

func TestCheckVersion(t *testing.T) {
	// Mock GitHub Releases API
	mockReleases := []githubRelease{
		{
			TagName:    "v0.1.9",
			Prerelease: false,
			HTMLURL:    "https://github.com/Hoosk/motoko/releases/tag/v0.1.9",
			Assets: []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			}{
				{Name: "motoko_linux_amd64.tar.gz", BrowserDownloadURL: "https://github.com/Hoosk/motoko/releases/download/v0.1.9/motoko_linux_amd64.tar.gz"},
				{Name: "motoko_darwin_amd64.tar.gz", BrowserDownloadURL: "https://github.com/Hoosk/motoko/releases/download/v0.1.9/motoko_darwin_amd64.tar.gz"},
			},
		},
		{
			TagName:    "v0.2.0-alpha",
			Prerelease: true,
			HTMLURL:    "https://github.com/Hoosk/motoko/releases/tag/v0.2.0-alpha",
			Assets: []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			}{
				{Name: "motoko_linux_amd64.tar.gz", BrowserDownloadURL: "https://github.com/Hoosk/motoko/releases/download/v0.2.0-alpha/motoko_linux_amd64.tar.gz"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockReleases)
	}))
	defer server.Close()

	t.Run("stable channel - finds stable update", func(t *testing.T) {
		upd := NewUpdater(Config{
			CurrentVersion: "v0.1.8",
			GOOS:           "linux",
			GOARCH:         "amd64",
		})
		upd.releasesURL = server.URL

		info, err := upd.CheckVersion(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !info.IsNewer {
			t.Error("expected update to be available")
		}
		if info.NewVersion != "v0.1.9" {
			t.Errorf("got version %q, want v0.1.9", info.NewVersion)
		}
		if info.DownloadURL != "https://github.com/Hoosk/motoko/releases/download/v0.1.9/motoko_linux_amd64.tar.gz" {
			t.Errorf("unexpected download URL: %q", info.DownloadURL)
		}
	})

	t.Run("alpha channel - finds pre-release update", func(t *testing.T) {
		upd := NewUpdater(Config{
			CurrentVersion: "v0.1.8-alpha",
			GOOS:           "linux",
			GOARCH:         "amd64",
		})
		upd.releasesURL = server.URL

		info, err := upd.CheckVersion(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !info.IsNewer {
			t.Error("expected update to be available")
		}
		// v0.2.0-alpha is higher than v0.1.9 and v0.1.8-alpha
		if info.NewVersion != "v0.2.0-alpha" {
			t.Errorf("got version %q, want v0.2.0-alpha", info.NewVersion)
		}
	})

	t.Run("up to date", func(t *testing.T) {
		upd := NewUpdater(Config{
			CurrentVersion: "v0.1.9",
			GOOS:           "linux",
			GOARCH:         "amd64",
		})
		upd.releasesURL = server.URL

		info, err := upd.CheckVersion(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.IsNewer {
			t.Error("expected no update to be available")
		}
	})

	t.Run("dev version ignores update", func(t *testing.T) {
		upd := NewUpdater(Config{
			CurrentVersion: "dev",
			GOOS:           "linux",
			GOARCH:         "amd64",
		})
		upd.releasesURL = server.URL

		info, err := upd.CheckVersion(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.IsNewer {
			t.Error("dev build should not have updates available")
		}
	})
}
