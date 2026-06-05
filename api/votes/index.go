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
		UserID    string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.CardID == "" || body.SessionID == "" || body.UserID == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	db := supa.New()

	// Check if vote exists
	var existing []models.DBCardVote
	query := "card_id=eq." + body.CardID + "&user_id=eq." + body.UserID
	if err := db.Select("card_votes", query, &existing); err != nil {
		http.Error(w, "failed to check vote: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(existing) > 0 {
		// Vote exists — delete (toggle off)
		if err := db.Delete("card_votes", query); err != nil {
			http.Error(w, "failed to remove vote: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"action": "removed"})
	} else {
		// Vote does not exist — insert
		voteData := map[string]interface{}{
			"card_id":    body.CardID,
			"session_id": body.SessionID,
			"user_id":    body.UserID,
		}
		if err := db.Insert("card_votes", voteData, nil); err != nil {
			http.Error(w, "failed to add vote: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"action": "added"})
	}
}
