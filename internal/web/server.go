package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/montanaflynn/botctl/internal/db"
	"github.com/montanaflynn/botctl/internal/service"
)

//go:embed static
var staticFiles embed.FS

// Serve starts the web dashboard HTTP server on the given port.
func Serve(port int) error {
	database, err := db.Open()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	h := &handler{svc: service.New(database)}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/bots", h.listBots)
	mux.HandleFunc("GET /api/bots/{name}", h.getBot)
	mux.HandleFunc("POST /api/bots/{name}/start", h.startBot)
	mux.HandleFunc("POST /api/bots/{name}/stop", h.stopBot)
	mux.HandleFunc("POST /api/bots/{name}/message", h.messageBot)
	mux.HandleFunc("POST /api/bots/{name}/resume", h.resumeBot)
	mux.HandleFunc("DELETE /api/bots/{name}", h.deleteBot)
	mux.HandleFunc("GET /api/bots/{name}/logs", h.getBotLogs)
	mux.HandleFunc("GET /api/bots/{name}/logs/stream", h.streamBotLogs)
	mux.HandleFunc("GET /api/stats", h.getStats)

	// Static files
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(static)))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Graceful shutdown on signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	fmt.Printf("botctl web dashboard: http://localhost:%d\n", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
