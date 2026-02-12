//go:build !windows

package update

import "os"

// replaceExecutable atomically replaces the running binary with the new one.
// On Unix this is a simple rename (atomic on the same filesystem).
func replaceExecutable(tmpPath, exePath string) error {
	return os.Rename(tmpPath, exePath)
}
