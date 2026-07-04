package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseVersion parses a semver string like "v1.2.3-alpha" or "1.2.0".
// It returns major, minor, patch, prerelease, and an error.
func ParseVersion(v string) (major, minor, patch int, prerelease string, err error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, 0, 0, "", fmt.Errorf("empty version string")
	}

	// Remove leading 'v' or 'V'
	if v[0] == 'v' || v[0] == 'V' {
		v = v[1:]
	}

	// Split version number and prerelease parts (separated by '-')
	parts := strings.SplitN(v, "-", 2)
	versionPart := parts[0]
	if len(parts) > 1 {
		prerelease = parts[1]
	}

	// Split major, minor, patch
	subParts := strings.Split(versionPart, ".")
	if len(subParts) == 0 || subParts[0] == "" {
		return 0, 0, 0, "", fmt.Errorf("invalid version format: %q", v)
	}

	major, err = strconv.Atoi(subParts[0])
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid major version %q: %w", subParts[0], err)
	}

	if len(subParts) > 1 {
		minor, err = strconv.Atoi(subParts[1])
		if err != nil {
			return 0, 0, 0, "", fmt.Errorf("invalid minor version %q: %w", subParts[1], err)
		}
	}

	if len(subParts) > 2 {
		patch, err = strconv.Atoi(subParts[2])
		if err != nil {
			return 0, 0, 0, "", fmt.Errorf("invalid patch version %q: %w", subParts[2], err)
		}
	}

	return major, minor, patch, prerelease, nil
}

// CompareVersions compares two semver strings: current and latest.
// Returns:
//
//	-1 if current < latest (latest is newer)
//	 0 if current == latest
//	 1 if current > latest
//
// If either version is invalid (e.g. "dev"), they are considered equal (returns 0).
func CompareVersions(current, latest string) int {
	curMajor, curMinor, curPatch, curPre, err1 := ParseVersion(current)
	latMajor, latMinor, latPatch, latPre, err2 := ParseVersion(latest)

	if err1 != nil || err2 != nil {
		return 0
	}

	if curMajor != latMajor {
		if curMajor < latMajor {
			return -1
		}
		return 1
	}

	if curMinor != latMinor {
		if curMinor < latMinor {
			return -1
		}
		return 1
	}

	if curPatch != latPatch {
		if curPatch < latPatch {
			return -1
		}
		return 1
	}

	// Compare prereleases
	// A release with NO prerelease is newer than one WITH a prerelease
	// e.g., v1.2.3 > v1.2.3-alpha
	if curPre == "" && latPre != "" {
		return 1
	}
	if curPre != "" && latPre == "" {
		return -1
	}
	if curPre == "" && latPre == "" {
		return 0
	}

	// Both have prereleases, compare lexicographically
	// e.g. "alpha" < "beta" < "rc"
	if curPre < latPre {
		return -1
	}
	if curPre > latPre {
		return 1
	}

	return 0
}
