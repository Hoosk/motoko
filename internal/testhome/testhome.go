// Package testhome keeps tests away from the developer's real per-user
// directories.
package testhome

import (
	"fmt"
	"os"
	"testing"
)

// Run runs a package's tests with HOME and the XDG/AppData variables pointing
// at a fresh temporary directory, so code resolving os.UserConfigDir,
// os.UserCacheDir or os.UserHomeDir (config, API key, model cache, sessions,
// brain) never reads or writes the real ones. Setting XDG_CONFIG_HOME alone
// is not enough: on macOS os.UserConfigDir ignores it.
//
// Use it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }
func Run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "motoko-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "AppData", "LocalAppData"} {
		if err := os.Setenv(key, dir); err != nil {
			fmt.Fprintln(os.Stderr, "testhome:", err)
			return 1
		}
	}
	return m.Run()
}
