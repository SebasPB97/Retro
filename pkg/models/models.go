package models

// DB row types (matching Supabase table columns)
type DBSession struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	HostID         string  `json:"host_id"`
	Phase          string  `json:"phase"`
	FocusedCardID  *string `json:"focused_card_id"`
	TimerDuration  *int    `json:"timer_duration"`
	TimerStartedAt *string `json:"timer_started_at"`
	TimerRunning   bool    `json:"timer_running"`
	CreatedAt      string  `json:"created_at"`
}

type DBColumn struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Order     int    `json:"order"`
}

type DBCard struct {
	ID        string  `json:"id"`
	SessionID string  `json:"session_id"`
	ColumnID  string  `json:"column_id"`
	Text      string  `json:"text"`
	Author    string  `json:"author"`
	AuthorID  string  `json:"author_id"`
	GroupID   *string `json:"group_id"`
	CreatedAt string  `json:"created_at"`
}

type DBCardVote struct {
	ID        string `json:"id"`
	CardID    string `json:"card_id"`
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
}

type DBComment struct {
	ID        string `json:"id"`
	CardID    string `json:"card_id"`
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type DBReaction struct {
	ID        string `json:"id"`
	CardID    string `json:"card_id"`
	SessionID string `json:"session_id"`
	Emoji     string `json:"emoji"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
}

type DBActionItem struct {
	ID        string `json:"id"`
	CardID    string `json:"card_id"`
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	Assignee  string `json:"assignee"`
	DueDate   string `json:"due_date"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
}

// API response types (matching frontend expectations from original app)
type SessionResponse struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	HostID        string       `json:"host_id"`
	Phase         string       `json:"phase"`
	FocusedCardID string       `json:"focused_card_id"`
	Timer         *TimerResp   `json:"timer,omitempty"`
	Columns       []ColumnResp `json:"columns"`
	Cards         []CardResp   `json:"cards"`
	CreatedAt     string       `json:"created_at"`
}

type TimerResp struct {
	Duration  int    `json:"duration"`
	StartedAt string `json:"started_at"`
	Running   bool   `json:"running"`
}

type ColumnResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Order int    `json:"order"`
}

type CardResp struct {
	ID        string         `json:"id"`
	ColumnID  string         `json:"column_id"`
	Text      string         `json:"text"`
	Author    string         `json:"author"`
	AuthorID  string         `json:"author_id"`
	Votes     []string       `json:"votes"`
	Comments  []CommentResp  `json:"comments"`
	Reactions []ReactionResp `json:"reactions"`
	Actions   []ActionResp   `json:"actions"`
	GroupID   string         `json:"group_id,omitempty"`
	CreatedAt string         `json:"created_at"`
}

type CommentResp struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type ReactionResp struct {
	ID       string `json:"id"`
	Emoji    string `json:"emoji"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type ActionResp struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Assignee  string `json:"assignee"`
	DueDate   string `json:"due_date"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
}
