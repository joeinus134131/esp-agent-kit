//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// App yang jalan dari autostart/Dock tidak mewarisi PATH shell,
// jadi tambahkan lokasi umum node/ccusage.
func init() {
	home, _ := os.UserHomeDir()
	extra := []string{
		"/opt/homebrew/bin", "/usr/local/bin",
		filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".npm-global/bin"),
	}
	if m, _ := filepath.Glob(filepath.Join(home, ".nvm/versions/node/*/bin")); len(m) > 0 {
		extra = append(extra, m...)
	}
	sep := string(os.PathListSeparator)
	os.Setenv("PATH", os.Getenv("PATH")+sep+strings.Join(extra, sep))
}
