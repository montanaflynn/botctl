package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/montanaflynn/botctl/pkg/paths"
	"github.com/montanaflynn/botctl/pkg/service"
	"github.com/montanaflynn/botctl/pkg/skills"
)

type handler struct {
	svc       *service.Service
	skillsSvc *skills.SkillsService
}

type botInfoJSON struct {
	Name      string  `json:"name"`
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	PID       int     `json:"pid"`
	Runs      int     `json:"runs"`
	Cost      float64 `json:"cost"`
	Turns     int     `json:"turns"`
	LastRun   string  `json:"last_run"`
	Interval  int     `json:"interval"`
	MaxTurns  int     `json:"max_turns"`
	Workspace string  `json:"workspace"`
}

type logEntryJSON struct {
	ID    int64    `json:"id"`
	Kind  string   `json:"kind"`
	Lines []string `json:"lines"`
}

type actionResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type statsResponse struct {
	TotalBots    int     `json:"total_bots"`
	ActiveBots   int     `json:"active_bots"`
	RunningBots  int     `json:"running_bots"`
	SleepingBots int     `json:"sleeping_bots"`
	PausedBots   int     `json:"paused_bots"`
	TotalRuns    int     `json:"total_runs"`
	TotalCost    float64 `json:"total_cost"`
	TotalTurns   int     `json:"total_turns"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, actionResponse{Error: msg})
}

func writeOK(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, actionResponse{OK: true, Message: msg})
}

func toBotInfoJSON(b service.BotInfo) botInfoJSON {
	info := botInfoJSON{
		Name:    b.Name,
		ID:      b.ID,
		Status:  b.Status,
		PID:     b.PID,
		Runs:    b.Stats.Runs,
		Cost:    b.Stats.TotalCost,
		Turns:   b.Stats.TotalTurns,
		LastRun: b.Stats.LastRun,
	}
	if b.Config != nil {
		info.Interval = b.Config.IntervalSeconds
		info.MaxTurns = b.Config.MaxTurns
		info.Workspace = b.Config.Workspace
	}
	return info
}

func (h *handler) listBots(w http.ResponseWriter, r *http.Request) {
	bots, _ := h.svc.ListBots("")
	result := make([]botInfoJSON, 0, len(bots))
	for _, b := range bots {
		result = append(result, toBotInfoJSON(b))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) getBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	bot, err := h.svc.GetBot(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "bot not found")
		return
	}
	writeJSON(w, http.StatusOK, toBotInfoJSON(bot))
}

func (h *handler) startBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pid, err := h.svc.StartBot(name)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	writeOK(w, fmt.Sprintf("started (pid %d)", pid))
}

func (h *handler) stopBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := h.svc.StopBot(name)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	writeOK(w, "stopped")
}

func (h *handler) messageBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	result, err := h.svc.SendMessage(name, body.Message)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeOK(w, result)
}

func (h *handler) resumeBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Turns int `json:"turns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Turns <= 0 {
		writeError(w, http.StatusBadRequest, "turns must be a positive integer")
		return
	}

	result, err := h.svc.Resume(name, body.Turns)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeOK(w, result)
}

func (h *handler) pauseBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := h.svc.PauseBot(name)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	writeOK(w, "pausing")
}

func (h *handler) playBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Turns int `json:"turns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Turns <= 0 {
		writeError(w, http.StatusBadRequest, "turns must be a positive integer")
		return
	}

	pid, err := h.svc.PlayBot(name, body.Turns)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	writeOK(w, fmt.Sprintf("playing (pid %d, %d turns)", pid, body.Turns))
}

func (h *handler) deleteBot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := h.svc.DeleteBot(name)
	if err != nil {
		if err == service.ErrBotNotFound {
			writeError(w, http.StatusNotFound, "bot not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeOK(w, fmt.Sprintf("%s deleted", name))
}

func (h *handler) getBotLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	bot, err := h.svc.GetBot(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "bot not found")
		return
	}

	entries := h.svc.RecentLogEntries(bot.ID, 500)
	result := make([]logEntryJSON, 0, len(entries))
	for _, e := range entries {
		lines := h.svc.RenderLogEntry(e)
		if lines == nil {
			continue
		}
		result = append(result, logEntryJSON{
			ID:    e.ID,
			Kind:  e.Kind,
			Lines: lines,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) getStats(w http.ResponseWriter, r *http.Request) {
	s := h.svc.GetStats()
	writeJSON(w, http.StatusOK, statsResponse{
		TotalBots:    s.TotalBots,
		ActiveBots:   s.ActiveBots,
		RunningBots:  s.RunningBots,
		SleepingBots: s.SleepingBots,
		PausedBots:   s.PausedBots,
		TotalRuns:    s.TotalRuns,
		TotalCost:    s.TotalCost,
		TotalTurns:   s.TotalTurns,
	})
}

// Skills handlers

func (h *handler) listSkills(w http.ResponseWriter, r *http.Request) {
	bot := r.URL.Query().Get("bot")
	found := h.skillsSvc.List(bot)
	type skillJSON struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
	}
	result := make([]skillJSON, 0, len(found))
	for _, s := range found {
		result = append(result, skillJSON{Name: s.Name, Description: s.Description, Source: s.Source})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) searchSkills(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	results, err := h.skillsSvc.Search(q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *handler) viewSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	repo := r.URL.Query().Get("repo")
	result, err := h.skillsSvc.View(name, repo)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) addSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source  string `json:"source"`
		Skill   string `json:"skill"`
		DestDir string `json:"dest_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	destDir := body.DestDir
	if destDir == "" {
		destDir = paths.GlobalSkillsDir()
	}
	installed, err := h.skillsSvc.Install(body.Source, skills.InstallOpts{
		SkillFilter: body.Skill,
		DestDir:     destDir,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, actionResponse{OK: true, Message: fmt.Sprintf("installed: %s", installed)})
}

func (h *handler) removeSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	removed, err := h.skillsSvc.Remove(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeOK(w, fmt.Sprintf("removed %s from %d location(s)", name, len(removed)))
}
