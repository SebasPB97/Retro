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
		createAction(w, r)
	case http.MethodPut:
		updateAction(w, r)
	case http.MethodDelete:
		deleteAction(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func createAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CardID    string `json:"card_id"`
		SessionID string `json:"session_id"`
		Text      string `json:"text"`
		Assignee  string `json:"assignee"`
		DueDate   string `json:"due_date"`
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
	actionData := map[string]interface{}{
		"card_id":    body.CardID,
		"session_id": body.SessionID,
		"text":       body.Text,
		"assignee":   body.Assignee,
		"due_date":   body.DueDate,
		"done":       false,
	}

	var created []models.DBActionItem
	if err := db.Insert("action_items", actionData, &created); err != nil {
		http.Error(w, "failed to create action item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if len(created) > 0 {
		json.NewEncoder(w).Encode(created[0])
	}
}

func updateAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string  `json:"id"`
		Done     *bool   `json:"done"`
		Text     *string `json:"text"`
		Assignee *string `json:"assignee"`
		DueDate  *string `json:"due_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		http.Error(w, "missing action id", http.StatusBadRequest)
		return
	}

	update := map[string]interface{}{}
	if body.Done != nil {
		update["done"] = *body.Done
	}
	if body.Text != nil {
		update["text"] = *body.Text
	}
	if body.Assignee != nil {
		update["assignee"] = *body.Assignee
	}
	if body.DueDate != nil {
		update["due_date"] = *body.DueDate
	}
	if len(update) == 0 {
		http.Error(w, "no fields to update", http.StatusBadRequest)
		return
	}

	db := supa.New()
	if err := db.Update("action_items", "id=eq."+body.ID, update); err != nil {
		http.Error(w, "failed to update action item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func deleteAction(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	sessionID := r.URL.Query().Get("sessionId")
	if id == "" || sessionID == "" {
		http.Error(w, "missing id or sessionId", http.StatusBadRequest)
		return
	}

	db := supa.New()
	if err := db.Delete("action_items", "id=eq."+id+"&session_id=eq."+sessionID); err != nil {
		http.Error(w, "failed to delete action item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
