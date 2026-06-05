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

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		CardID    string `json:"card_id"`
		SessionID string `json:"session_id"`
		Emoji     string `json:"emoji"`
		UserID    string `json:"user_id"`
		Username  string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.CardID == "" || body.SessionID == "" || body.Emoji == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	db := supa.New()

	// Check if reaction exists (toggle)
	var existing []models.DBReaction
	query := "card_id=eq." + body.CardID + "&user_id=eq." + body.UserID + "&emoji=eq." + body.Emoji
	if err := db.Select("reactions", query, &existing); err != nil {
		http.Error(w, "failed to check reaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(existing) > 0 {
		// Reaction exists — delete it
		if err := db.Delete("reactions", "id=eq."+existing[0].ID); err != nil {
			http.Error(w, "failed to remove reaction: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"action": "removed"})
	} else {
		// Reaction does not exist — insert
		reactionData := map[string]interface{}{
			"card_id":    body.CardID,
			"session_id": body.SessionID,
			"emoji":      body.Emoji,
			"user_id":    body.UserID,
			"username":   body.Username,
		}
		var created []models.DBReaction
		if err := db.Insert("reactions", reactionData, &created); err != nil {
			http.Error(w, "failed to add reaction: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"action": "added"})
	}
}
