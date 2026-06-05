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
		createCard(w, r)
	case http.MethodPut:
		updateCard(w, r)
	case http.MethodDelete:
		deleteCard(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func createCard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
		ColumnID  string `json:"column_id"`
		Text      string `json:"text"`
		Author    string `json:"author"`
		AuthorID  string `json:"author_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.SessionID == "" || body.ColumnID == "" || body.Text == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	db := supa.New()
	cardData := map[string]interface{}{
		"session_id": body.SessionID,
		"column_id":  body.ColumnID,
		"text":       body.Text,
		"author":     body.Author,
		"author_id":  body.AuthorID,
	}

	var created []models.DBCard
	if err := db.Insert("cards", cardData, &created); err != nil {
		http.Error(w, "failed to create card: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if len(created) > 0 {
		json.NewEncoder(w).Encode(created[0])
	}
}

func updateCard(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	id, ok := body["id"].(string)
	if !ok || id == "" {
		http.Error(w, "missing card id", http.StatusBadRequest)
		return
	}

	// Build partial update — only include fields that are present in the request body
	update := map[string]interface{}{}
	if text, ok := body["text"]; ok {
		update["text"] = text
	}
	if columnID, ok := body["column_id"]; ok {
		update["column_id"] = columnID
	}
	if groupID, exists := body["group_id"]; exists {
		// groupID can be null (to ungroup)
		update["group_id"] = groupID
	}

	if len(update) == 0 {
		http.Error(w, "no fields to update", http.StatusBadRequest)
		return
	}

	db := supa.New()
	if err := db.Update("cards", "id=eq."+id, update); err != nil {
		http.Error(w, "failed to update card: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func deleteCard(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("id")
	userID := r.URL.Query().Get("userId")
	sessionID := r.URL.Query().Get("sessionId")

	if cardID == "" || userID == "" || sessionID == "" {
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	db := supa.New()

	// Fetch the card to verify ownership
	var cards []models.DBCard
	if err := db.Select("cards", "id=eq."+cardID+"&session_id=eq."+sessionID, &cards); err != nil {
		http.Error(w, "failed to fetch card: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(cards) == 0 {
		http.Error(w, "card not found", http.StatusNotFound)
		return
	}
	card := cards[0]

	// Check if user is the author
	if card.AuthorID != userID {
		// Check if user is the session host
		var sessions []models.DBSession
		if err := db.Select("sessions", "id=eq."+sessionID, &sessions); err != nil || len(sessions) == 0 || sessions[0].HostID != userID {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
	}

	if err := db.Delete("cards", "id=eq."+cardID); err != nil {
		http.Error(w, "failed to delete card: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
