package website

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Serve starts a dev server on the given port. It re-reads and injects
// helpText on every request so edits to source files are reflected on refresh.
func Serve(dir string, port int, helpText string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Resolve file path: /docs/ -> docs/index.html, /foo -> foo
		filePath := filepath.Join(dir, r.URL.Path)
		if info, err := os.Stat(filePath); err == nil && info.IsDir() {
			filePath = filepath.Join(filePath, "index.html")
		}

		// Skip dist directory
		rel, _ := filepath.Rel(dir, filePath)
		if rel == "dist" || strings.HasPrefix(rel, "dist/") {
			http.NotFound(w, r)
			return
		}

		// For HTML files, read, inject, and serve
		if strings.HasSuffix(filePath, ".html") {
			raw, err := os.ReadFile(filePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}

			result := InjectCommit(InjectHelp(string(raw), helpText))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(result))
			return
		}

		// Non-HTML: serve directly
		http.ServeFile(w, r, filePath)
	})

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Serving %s at http://localhost%s\n", dir, addr)
	return http.ListenAndServe(addr, mux)
}
