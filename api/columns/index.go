package handler

import (
	"encoding/json"
	"net/http"
	"retro-tool-vercel/pkg/cors"
	"retro-tool-vercel/pkg/models"
	"retro-tool-vercel/pkg/supa"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if cors.Preflight(w, r) {
		return
	}
	cors.Set(w)

	switch r.Method {
	case http.MethodPost:
		createColumn(w, r)
	case http.MethodDelete:
		deleteColumn(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func createColumn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
		Name      string `json:"name"`
		Color     string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.SessionID == "" || body.Name == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	if body.Color == "" {
		body.Color = "#9c27b0"
	}

	db := supa.New()

	// Get current max order for the session
	var cols []models.DBColumn
	_ = db.Select("columns", `session_id=eq.`+body.SessionID+`&order="order".desc&limit=1`, &cols)
	nextOrder := 0
	if len(cols) > 0 {
		nextOrder = cols[0].Order + 1
	}

	colData := map[string]interface{}{
		"session_id": body.SessionID,
		"name":       body.Name,
		"color":      body.Color,
		"order":      nextOrder,
	}

	var created []models.DBColumn
	if err := db.Insert("columns", colData, &created); err != nil {
		http.Error(w, "failed to create column: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if len(created) > 0 {
		json.NewEncoder(w).Encode(created[0])
	}
}

func deleteColumn(w http.ResponseWriter, r *http.Request) {
	columnID := r.URL.Query().Get("id")
	sessionID := r.URL.Query().Get("sessionId")

	if columnID == "" || sessionID == "" {
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	db := supa.New()

	// Verify the column belongs to the session
	var cols []models.DBColumn
	if err := db.Select("columns", "id=eq."+columnID+"&session_id=eq."+sessionID, &cols); err != nil || len(cols) == 0 {
		http.Error(w, "column not found", http.StatusNotFound)
		return
	}

	if err := db.Delete("columns", "id=eq."+columnID); err != nil {
		http.Error(w, "failed to delete column: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
