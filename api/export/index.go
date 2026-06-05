package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"retro-tool-vercel/internal/cors"
	"retro-tool-vercel/internal/models"
	"retro-tool-vercel/internal/supa"
	"sort"
	"strings"
	"time"
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

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "missing sessionId parameter", http.StatusBadRequest)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	sess, err := fetchFullSession(sessionID)
	if err != nil {
		http.Error(w, "failed to fetch session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="retro-%s.json"`, sess.ID))
		json.NewEncoder(w).Encode(sess)
	case "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="retro-%s.md"`, sess.ID))
		fmt.Fprint(w, toMarkdown(sess))
	default:
		http.Error(w, "unsupported format", http.StatusBadRequest)
	}
}

func fetchFullSession(sessionID string) (*models.SessionResponse, error) {
	db := supa.New()

	var sessions []models.DBSession
	if err := db.Select("sessions", "id=eq."+sessionID, &sessions); err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	sess := sessions[0]

	var columns []models.DBColumn
	if err := db.Select("columns", `session_id=eq.`+sessionID+`&order="order".asc`, &columns); err != nil {
		return nil, err
	}

	var cards []models.DBCard
	if err := db.Select("cards", "session_id=eq."+sessionID+"&order=created_at.asc", &cards); err != nil {
		return nil, err
	}

	var votes []models.DBCardVote
	if err := db.Select("card_votes", "session_id=eq."+sessionID, &votes); err != nil {
		return nil, err
	}

	var comments []models.DBComment
	if err := db.Select("comments", "session_id=eq."+sessionID+"&order=created_at.asc", &comments); err != nil {
		return nil, err
	}

	var reactions []models.DBReaction
	if err := db.Select("reactions", "session_id=eq."+sessionID, &reactions); err != nil {
		return nil, err
	}

	var actionItems []models.DBActionItem
	if err := db.Select("action_items", "session_id=eq."+sessionID+"&order=created_at.asc", &actionItems); err != nil {
		return nil, err
	}

	// Build lookup maps
	votesByCard := map[string][]string{}
	for _, v := range votes {
		votesByCard[v.CardID] = append(votesByCard[v.CardID], v.UserID)
	}
	commentsByCard := map[string][]models.CommentResp{}
	for _, c := range comments {
		commentsByCard[c.CardID] = append(commentsByCard[c.CardID], models.CommentResp{
			ID: c.ID, Text: c.Text, UserID: c.UserID, Username: c.Username, CreatedAt: c.CreatedAt,
		})
	}
	reactionsByCard := map[string][]models.ReactionResp{}
	for _, rx := range reactions {
		reactionsByCard[rx.CardID] = append(reactionsByCard[rx.CardID], models.ReactionResp{
			ID: rx.ID, Emoji: rx.Emoji, UserID: rx.UserID, Username: rx.Username,
		})
	}
	actionsByCard := map[string][]models.ActionResp{}
	for _, a := range actionItems {
		actionsByCard[a.CardID] = append(actionsByCard[a.CardID], models.ActionResp{
			ID: a.ID, Text: a.Text, Assignee: a.Assignee, DueDate: a.DueDate, Done: a.Done, CreatedAt: a.CreatedAt,
		})
	}

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
			ID: c.ID, ColumnID: c.ColumnID, Text: c.Text,
			Author: c.Author, AuthorID: c.AuthorID, GroupID: groupID,
			Votes: vs, Comments: cs, Reactions: rs, Actions: as, CreatedAt: c.CreatedAt,
		})
	}

	colResps := make([]models.ColumnResp, 0, len(columns))
	for _, col := range columns {
		colResps = append(colResps, models.ColumnResp{ID: col.ID, Name: col.Name, Color: col.Color, Order: col.Order})
	}

	focusedCardID := ""
	if sess.FocusedCardID != nil {
		focusedCardID = *sess.FocusedCardID
	}

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
		timer = &models.TimerResp{Duration: dur, StartedAt: startedAt, Running: sess.TimerRunning}
	}

	return &models.SessionResponse{
		ID:            sess.ID,
		Name:          sess.Name,
		HostID:        sess.HostID,
		Phase:         sess.Phase,
		FocusedCardID: focusedCardID,
		Timer:         timer,
		Columns:       colResps,
		Cards:         cardResps,
		CreatedAt:     sess.CreatedAt,
	}, nil
}

func toMarkdown(sess *models.SessionResponse) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", sess.Name))

	createdDate := sess.CreatedAt
	if len(createdDate) >= 10 {
		createdDate = createdDate[:10]
	}
	sb.WriteString(fmt.Sprintf("**Date:** %s  \n", createdDate))
	sb.WriteString(fmt.Sprintf("**Phase:** %s  \n\n", sess.Phase))

	// Sort columns by order
	cols := make([]models.ColumnResp, len(sess.Columns))
	copy(cols, sess.Columns)
	sort.Slice(cols, func(i, j int) bool {
		return cols[i].Order < cols[j].Order
	})

	for _, col := range cols {
		sb.WriteString(fmt.Sprintf("## %s\n\n", col.Name))

		// Collect cards for this column, sorted by votes desc
		var colCards []models.CardResp
		for _, c := range sess.Cards {
			if c.ColumnID == col.ID {
				colCards = append(colCards, c)
			}
		}
		sort.Slice(colCards, func(i, j int) bool {
			return len(colCards[i].Votes) > len(colCards[j].Votes)
		})

		if len(colCards) == 0 {
			sb.WriteString("_No cards_\n\n")
			continue
		}

		for _, card := range colCards {
			votes := len(card.Votes)
			voteStr := ""
			if votes > 0 {
				voteStr = fmt.Sprintf(" (%d votes)", votes)
			}
			sb.WriteString(fmt.Sprintf("### %s%s\n", card.Text, voteStr))

			cardDate := card.CreatedAt
			if len(cardDate) >= 16 {
				cardDate = cardDate[:16]
			}
			sb.WriteString(fmt.Sprintf("*Author: %s — %s*\n\n", card.Author, cardDate))

			// Reactions
			if len(card.Reactions) > 0 {
				reactionCounts := make(map[string]int)
				for _, rx := range card.Reactions {
					reactionCounts[rx.Emoji]++
				}
				sb.WriteString("**Reactions:** ")
				first := true
				for emoji, count := range reactionCounts {
					if !first {
						sb.WriteString(" ")
					}
					sb.WriteString(fmt.Sprintf("%s×%d", emoji, count))
					first = false
				}
				sb.WriteString("\n\n")
			}

			// Comments
			if len(card.Comments) > 0 {
				sb.WriteString("**Comments:**\n")
				for _, c := range card.Comments {
					commentTime := c.CreatedAt
					if len(commentTime) >= 16 {
						commentTime = commentTime[11:16]
					}
					sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", c.Username, commentTime, c.Text))
				}
				sb.WriteString("\n")
			}

			// Action items
			if len(card.Actions) > 0 {
				sb.WriteString("**Action Items:**\n")
				for _, a := range card.Actions {
					done := " "
					if a.Done {
						done = "x"
					}
					assignee := ""
					if a.Assignee != "" {
						assignee = fmt.Sprintf(" → @%s", a.Assignee)
					}
					dueDate := ""
					if a.DueDate != "" {
						dueDate = fmt.Sprintf(" (due: %s)", a.DueDate)
					}
					sb.WriteString(fmt.Sprintf("- [%s] %s%s%s\n", done, a.Text, assignee, dueDate))
				}
				sb.WriteString("\n")
			}
		}
	}

	// Summary of all action items
	type actionWithCard struct {
		CardText string
		Action   models.ActionResp
	}
	var allActions []actionWithCard
	for _, card := range sess.Cards {
		for _, a := range card.Actions {
			allActions = append(allActions, actionWithCard{CardText: card.Text, Action: a})
		}
	}
	if len(allActions) > 0 {
		sb.WriteString("---\n\n## Action Items Summary\n\n")
		for _, item := range allActions {
			done := " "
			if item.Action.Done {
				done = "x"
			}
			assignee := ""
			if item.Action.Assignee != "" {
				assignee = fmt.Sprintf(" → @%s", item.Action.Assignee)
			}
			dueDate := ""
			if item.Action.DueDate != "" {
				dueDate = fmt.Sprintf(" (due: %s)", item.Action.DueDate)
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s%s%s *(from: %s)*\n",
				done, item.Action.Text, assignee, dueDate, item.CardText))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("\n---\n*Exported on %s*\n", time.Now().Format("2006-01-02 15:04:05")))
	return sb.String()
}
