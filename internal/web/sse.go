package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/montanaflynn/botctl-go/internal/service"
)

func (h *handler) streamBotLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	bot, err := h.svc.GetBot(name)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Start from a given ID if provided via query param
	var lastID int64
	if after := r.URL.Query().Get("after"); after != "" {
		lastID, _ = strconv.ParseInt(after, 10, 64)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			entries := h.svc.LogEntriesAfter(bot.ID, lastID, 500)
			for _, e := range entries {
				lines := h.svc.RenderLogEntry(e)
				if lines == nil {
					continue
				}
				entry := logEntryJSON{
					ID:    e.ID,
					Kind:  e.Kind,
					Lines: lines,
				}
				data, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				if e.ID > lastID {
					lastID = e.ID
				}
			}
			flusher.Flush()
		}
	}
}
