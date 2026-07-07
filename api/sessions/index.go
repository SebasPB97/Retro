package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"retro-tool-vercel/pkg/cors"
	"retro-tool-vercel/pkg/models"
	"retro-tool-vercel/pkg/supa"
	"time"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if cors.Preflight(w, r) {
		return
	}
	cors.Set(w)

	switch r.Method {
	case http.MethodGet:
		listSessions(w, r)
	case http.MethodPost:
		createSession(w, r)
	case http.MethodPut:
		updateSession(w, r)
	case http.MethodDelete:
		deleteSession(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listSessions(w http.ResponseWriter, r *http.Request) {
	db := supa.New()

	var sessions []models.DBSession
	if err := db.Select("sessions", "order=created_at.desc", &sessions); err != nil {
		http.Error(w, "failed to fetch sessions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type summary struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Phase     string `json:"phase"`
		CardCount int    `json:"card_count"`
		CreatedAt string `json:"created_at"`
	}

	result := make([]summary, 0, len(sessions))
	for _, s := range sessions {
		var cards []models.DBCard
		_ = db.Select("cards", "session_id=eq."+s.ID+"&select=id", &cards)
		result = append(result, summary{
			ID:        s.ID,
			Name:      s.Name,
			Phase:     s.Phase,
			CardCount: len(cards),
			CreatedAt: s.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Columns []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if body.Name == "" {
		body.Name = "Retrospectiva " + time.Now().Format("2006-01-02")
	}

	// Generate random host ID
	hostBytes := make([]byte, 16)
	if _, err := rand.Read(hostBytes); err != nil {
		http.Error(w, "failed to generate host id", http.StatusInternalServerError)
		return
	}
	hostID := hex.EncodeToString(hostBytes)

	db := supa.New()

	// Insert session
	newSession := map[string]interface{}{
		"name":    body.Name,
		"host_id": hostID,
		"phase":   "adding",
	}
	var createdSessions []models.DBSession
	if err := db.Insert("sessions", newSession, &createdSessions); err != nil {
		http.Error(w, "failed to create session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(createdSessions) == 0 {
		http.Error(w, "session creation returned no data", http.StatusInternalServerError)
		return
	}
	sessionID := createdSessions[0].ID

	// Determine columns to insert
	type colInput struct {
		Name  string
		Color string
	}
	var cols []colInput

	if len(body.Columns) > 0 {
		for _, c := range body.Columns {
			if c.Name != "" {
				cols = append(cols, colInput{Name: c.Name, Color: c.Color})
			}
		}
	}

	if len(cols) == 0 {
		cols = []colInput{
			{Name: "Me gustó", Color: "#4caf50"},
			{Name: "Me bloqueó", Color: "#f44336"},
			{Name: "Qué puede mejorar", Color: "#ff9800"},
			{Name: "Qué odié", Color: "#e91e63"},
		}
	}

	for i, col := range cols {
		colData := map[string]interface{}{
			"session_id": sessionID,
			"name":       col.Name,
			"color":      col.Color,
			"order":      i,
		}
		if err := db.Insert("columns", colData, nil); err != nil {
			http.Error(w, "failed to create column: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id":   sessionID,
		"host_user_id": hostID,
	})
}

// DELETE /api/sessions — permanently delete a session and all its data
func deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	db := supa.New()
	// Delete in dependency order to avoid FK violations
	for _, table := range []string{"action_items", "reactions", "comments", "card_votes", "cards", "columns"} {
		if err := db.Delete(table, "session_id=eq."+id); err != nil {
			http.Error(w, "failed to delete "+table+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := db.Delete("sessions", "id=eq."+id); err != nil {
		http.Error(w, "failed to delete session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/sessions — update session fields (phase, focused_card_id)
func updateSession(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	id, ok := body["id"].(string)
	if !ok || id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	// Build partial update — only include known, explicitly provided fields
	update := map[string]interface{}{}
	if phase, ok := body["phase"]; ok {
		update["phase"] = phase
	}
	// focused_card_id can be null (close focus) or a UUID string
	if _, exists := body["focused_card_id"]; exists {
		update["focused_card_id"] = body["focused_card_id"]
	}

	if len(update) == 0 {
		http.Error(w, "no fields to update", http.StatusBadRequest)
		return
	}

	db := supa.New()
	if err := db.Update("sessions", "id=eq."+id, update); err != nil {
		http.Error(w, "failed to update session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
