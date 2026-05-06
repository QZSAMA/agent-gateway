package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/agent-gateway/gateway/internal/model"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		metadata TEXT DEFAULT '{}'
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		metadata TEXT DEFAULT '{}',
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'submitted',
		input TEXT,
		output TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS approvals (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		action_type TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		risk_level TEXT NOT NULL DEFAULT 'low',
		created_at DATETIME NOT NULL,
		expires_at DATETIME,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_agent_id ON sessions(agent_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_provider ON sessions(provider);
	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_session_id ON tasks(session_id);
	CREATE INDEX IF NOT EXISTS idx_approvals_session_id ON approvals(session_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) SaveSession(sess *model.Session) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, agent_id, provider, status, created_at, updated_at, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.AgentID, sess.Provider, string(sess.Status),
		sess.CreatedAt, sess.UpdatedAt, "{}",
	)
	return err
}

func (s *Store) GetSession(id string) (*model.Session, error) {
	row := s.db.QueryRow(
		`SELECT id, agent_id, provider, status, created_at, updated_at FROM sessions WHERE id = ?`, id,
	)
	var sess model.Session
	var status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&sess.ID, &sess.AgentID, &sess.Provider, &status, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	sess.Status = model.SessionStatus(status)
	sess.CreatedAt = createdAt
	sess.UpdatedAt = updatedAt
	return &sess, nil
}

func (s *Store) ListSessions(agentID string) ([]model.Session, error) {
	var rows *sql.Rows
	var err error
	if agentID != "" {
		rows, err = s.db.Query(
			`SELECT id, agent_id, provider, status, created_at, updated_at FROM sessions WHERE agent_id = ? ORDER BY updated_at DESC`, agentID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, agent_id, provider, status, created_at, updated_at FROM sessions ORDER BY updated_at DESC`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var sess model.Session
		var status string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&sess.ID, &sess.AgentID, &sess.Provider, &status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		sess.Status = model.SessionStatus(status)
		sess.CreatedAt = createdAt
		sess.UpdatedAt = updatedAt
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) UpdateSessionStatus(id string, status model.SessionStatus) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now(), id,
	)
	return err
}

func (s *Store) SaveMessage(msg *model.Message) error {
	contentJSON := marshalJSON(msg.Content)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO messages (id, session_id, role, content, timestamp, metadata)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, string(msg.Role), contentJSON, msg.Timestamp, "{}",
	)
	return err
}

func (s *Store) GetHistory(sessionID string) ([]model.Message, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, role, content, timestamp FROM messages WHERE session_id = ? ORDER BY timestamp ASC`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var msg model.Message
		var role string
		var contentJSON string
		var timestamp time.Time
		if err := rows.Scan(&msg.ID, &msg.SessionID, &role, &contentJSON, &timestamp); err != nil {
			return nil, err
		}
		msg.Role = model.MessageRole(role)
		msg.Content = unmarshalContentBlocks(contentJSON)
		msg.Timestamp = timestamp
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *Store) SaveApproval(req *model.ApprovalRequest) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO approvals (id, session_id, agent_id, action_type, title, description, risk_level, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.SessionID, req.AgentID, req.ActionType, req.Title, req.Description, req.RiskLevel, req.CreatedAt, req.ExpiresAt,
	)
	return err
}
