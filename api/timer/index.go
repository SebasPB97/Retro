package handler

import (
	"encoding/json"
	"net/http"
	"retro-tool-vercel/internal/cors"
	"retro-tool-vercel/internal/supa"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if cors.Preflight(w, r) {
		return
	}
	cors.Set(w)

	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
		Duration  *int   `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.SessionID == "" || body.Action == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	db := supa.New()

	switch body.Action {
	case "start":
		dur := 600 // default 10 minutes
		if body.Duration != nil {
			dur = *body.Duration
		}
		update := map[string]interface{}{
			"timer_running":    true,
			"timer_duration":   dur,
			"timer_started_at": "now()",
		}
		if err := db.Update("sessions", "id=eq."+body.SessionID, update); err != nil {
			http.Error(w, "failed to start timer: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case "stop":
		update := map[string]interface{}{
			"timer_running": false,
		}
		if err := db.Update("sessions", "id=eq."+body.SessionID, update); err != nil {
			http.Error(w, "failed to stop timer: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "invalid action, must be 'start' or 'stop'", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
