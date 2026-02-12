//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
)

// replaceExecutable replaces the running binary on Windows.
// Windows locks running executables, so we rename the old one out of the way first,
// then move the new binary into place, and spawn a background cleanup process.
func replaceExecutable(tmpPath, exePath string) error {
	oldPath := exePath + ".old"

	// Remove any leftover .old file from a previous update
	os.Remove(oldPath)

	// Rename current executable out of the way
	if err := os.Rename(exePath, oldPath); err != nil {
		return err
	}

	// Move new binary into place
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Try to restore the old binary
		os.Rename(oldPath, exePath)
		return err
	}

	// Spawn a detached cleanup process that waits ~1s then deletes the old binary.
	// Uses ping as a portable sleep mechanism on Windows.
	cleanupCmd := `ping -n 2 127.0.0.1 >nul & del "` + filepath.ToSlash(oldPath) + `"`
	exec.Command("cmd", "/c", cleanupCmd).Start()

	return nil
}
