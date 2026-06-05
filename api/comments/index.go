package handler

import (
	"encoding/json"
	"net/http"
	"retro-tool-vercel/internal/cors"
	"retro-tool-vercel/internal/models"
	"retro-tool-vercel/internal/supa"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if cors.Preflight(w, r) {
		return
	}
	cors.Set(w)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		CardID    string `json:"card_id"`
		SessionID string `json:"session_id"`
		Text      string `json:"text"`
		UserID    string `json:"user_id"`
		Username  string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.CardID == "" || body.SessionID == "" || body.Text == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	db := supa.New()
	commentData := map[string]interface{}{
		"card_id":    body.CardID,
		"session_id": body.SessionID,
		"text":       body.Text,
		"user_id":    body.UserID,
		"username":   body.Username,
	}

	var created []models.DBComment
	if err := db.Insert("comments", commentData, &created); err != nil {
		http.Error(w, "failed to create comment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if len(created) > 0 {
		json.NewEncoder(w).Encode(created[0])
	}
}
