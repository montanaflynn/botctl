package website

import (
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var preRe = regexp.MustCompile(`(<pre id="cli-help">)([\s\S]*?)(</pre>)`)

// InjectHelp replaces the <pre id="cli-help"> content with HTML-escaped helpText.
// Idempotent — works on fresh or already-injected HTML.
func InjectHelp(srcHTML, helpText string) string {
	escaped := html.EscapeString(helpText)
	return preRe.ReplaceAllStringFunc(srcHTML, func(match string) string {
		parts := preRe.FindStringSubmatch(match)
		return parts[1] + escaped + parts[3]
	})
}

// Build copies the entire source dir to outDir, processing HTML files
// through InjectHelp. The dist directory is placed alongside the source
// by default (e.g. website/dist/).
func Build(srcDir, outDir, helpText string) error {
	// Clean previous build
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clean %s: %w", outDir, err)
	}

	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(srcDir, path)

		// Skip the dist directory itself
		if rel == "dist" || strings.HasPrefix(rel, "dist/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dst := filepath.Join(outDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}

		// Process HTML files through injection
		if strings.HasSuffix(path, ".html") {
			data = []byte(InjectHelp(string(data), helpText))
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", rel, err)
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
