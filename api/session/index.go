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

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	db := supa.New()

	// 1. Fetch session
	var sessions []models.DBSession
	if err := db.Select("sessions", "id=eq."+sessionID, &sessions); err != nil {
		http.Error(w, "failed to fetch session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(sessions) == 0 {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sess := sessions[0]

	// 2. Fetch columns ordered by "order"
	var columns []models.DBColumn
	if err := db.Select("columns", "session_id=eq."+sessionID+`&order="order".asc`, &columns); err != nil {
		http.Error(w, "failed to fetch columns: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Fetch cards
	var cards []models.DBCard
	if err := db.Select("cards", "session_id=eq."+sessionID+"&order=created_at.asc", &cards); err != nil {
		http.Error(w, "failed to fetch cards: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Fetch all votes for session
	var votes []models.DBCardVote
	if err := db.Select("card_votes", "session_id=eq."+sessionID, &votes); err != nil {
		http.Error(w, "failed to fetch votes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 5. Fetch all comments
	var comments []models.DBComment
	if err := db.Select("comments", "session_id=eq."+sessionID+"&order=created_at.asc", &comments); err != nil {
		http.Error(w, "failed to fetch comments: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 6. Fetch all reactions
	var reactions []models.DBReaction
	if err := db.Select("reactions", "session_id=eq."+sessionID, &reactions); err != nil {
		http.Error(w, "failed to fetch reactions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 7. Fetch all action items
	var actionItems []models.DBActionItem
	if err := db.Select("action_items", "session_id=eq."+sessionID+"&order=created_at.asc", &actionItems); err != nil {
		http.Error(w, "failed to fetch action items: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build lookup maps indexed by card_id
	votesByCard := map[string][]string{}
	for _, v := range votes {
		votesByCard[v.CardID] = append(votesByCard[v.CardID], v.UserID)
	}

	commentsByCard := map[string][]models.CommentResp{}
	for _, c := range comments {
		commentsByCard[c.CardID] = append(commentsByCard[c.CardID], models.CommentResp{
			ID:        c.ID,
			Text:      c.Text,
			UserID:    c.UserID,
			Username:  c.Username,
			CreatedAt: c.CreatedAt,
		})
	}

	reactionsByCard := map[string][]models.ReactionResp{}
	for _, rx := range reactions {
		reactionsByCard[rx.CardID] = append(reactionsByCard[rx.CardID], models.ReactionResp{
			ID:       rx.ID,
			Emoji:    rx.Emoji,
			UserID:   rx.UserID,
			Username: rx.Username,
		})
	}

	actionsByCard := map[string][]models.ActionResp{}
	for _, a := range actionItems {
		actionsByCard[a.CardID] = append(actionsByCard[a.CardID], models.ActionResp{
			ID:        a.ID,
			Text:      a.Text,
			Assignee:  a.Assignee,
			DueDate:   a.DueDate,
			Done:      a.Done,
			CreatedAt: a.CreatedAt,
		})
	}

	// Assemble card responses
	cardResps := make([]models.CardResp, 0, len(cards))
	for _, c := range cards {
		groupID := ""
		if c.GroupID != nil {
			groupID = *c.GroupID
		}

		vs := votesByCard[c.ID]
		if vs == nil {
			vs = []string{}
		}
		cs := commentsByCard[c.ID]
		if cs == nil {
			cs = []models.CommentResp{}
		}
		rs := reactionsByCard[c.ID]
		if rs == nil {
			rs = []models.ReactionResp{}
		}
		as := actionsByCard[c.ID]
		if as == nil {
			as = []models.ActionResp{}
		}

		cardResps = append(cardResps, models.CardResp{
			ID:        c.ID,
			ColumnID:  c.ColumnID,
			Text:      c.Text,
			Author:    c.Author,
			AuthorID:  c.AuthorID,
			GroupID:   groupID,
			Votes:     vs,
			Comments:  cs,
			Reactions: rs,
			Actions:   as,
			CreatedAt: c.CreatedAt,
		})
	}

	// Assemble column responses
	colResps := make([]models.ColumnResp, 0, len(columns))
	for _, col := range columns {
		colResps = append(colResps, models.ColumnResp{
			ID:    col.ID,
			Name:  col.Name,
			Color: col.Color,
			Order: col.Order,
		})
	}

	// Assemble focused card ID
	focusedCardID := ""
	if sess.FocusedCardID != nil {
		focusedCardID = *sess.FocusedCardID
	}

	// Assemble timer
	var timer *models.TimerResp
	if sess.TimerDuration != nil || sess.TimerRunning {
		dur := 0
		if sess.TimerDuration != nil {
			dur = *sess.TimerDuration
		}
		startedAt := ""
		if sess.TimerStartedAt != nil {
			startedAt = *sess.TimerStartedAt
		}
		timer = &models.TimerResp{
			Duration:  dur,
			StartedAt: startedAt,
			Running:   sess.TimerRunning,
		}
	}

	resp := models.SessionResponse{
		ID:            sess.ID,
		Name:          sess.Name,
		HostID:        sess.HostID,
		Phase:         sess.Phase,
		FocusedCardID: focusedCardID,
		Timer:         timer,
		Columns:       colResps,
		Cards:         cardResps,
		CreatedAt:     sess.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
