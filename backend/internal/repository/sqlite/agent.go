package sqlite

import (
	"context"
	"fmt"

	"github.com/quiztrace/quiztrace/internal/domain"
)

func (r *Repository) GetAgentSession(ctx context.Context, id int64) (*domain.AgentSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, context_json, status, created_at, updated_at FROM agent_sessions WHERE id = ?`, id)
	var s domain.AgentSession
	var ctxJSON, status string
	var createdAt, updatedAt string
	if err := row.Scan(&s.ID, &s.UserID, &s.Title, &ctxJSON, &status, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	s.ContextJSON = ctxJSON
	s.Status = status
	s.CreatedAt = parseSQLiteTime(createdAt)
	s.UpdatedAt = parseSQLiteTime(updatedAt)
	return &s, nil
}

func (r *Repository) ListAgentSessions(ctx context.Context, userID int64) ([]*domain.AgentSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, context_json, status, created_at, updated_at FROM agent_sessions
		WHERE user_id = ? ORDER BY updated_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.AgentSession{}
	for rows.Next() {
		var s domain.AgentSession
		var ctxJSON, status string
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &ctxJSON, &status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.ContextJSON = ctxJSON
		s.Status = status
		s.CreatedAt = parseSQLiteTime(createdAt)
		s.UpdatedAt = parseSQLiteTime(updatedAt)
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteAgentSession(ctx context.Context, userID, sessionID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM agent_sessions WHERE id = ? AND user_id = ?`, sessionID, userID)
	return err
}

func (r *Repository) SaveAgentMessage(ctx context.Context, sessionID int64, role, content, toolCallsJSON string) (*domain.AgentMessage, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_messages (session_id, role, content, tool_calls_json) VALUES (?, ?, ?, ?)`,
		sessionID, role, content, toolCallsJSON)
	if err != nil {
		return nil, fmt.Errorf("save agent message: %w", err)
	}
	id, _ := res.LastInsertId()
	_, _ = r.db.ExecContext(ctx, `UPDATE agent_sessions SET updated_at = datetime('now') WHERE id = ?`, sessionID)
	return r.GetAgentMessage(ctx, id)
}

func (r *Repository) GetAgentMessage(ctx context.Context, id int64) (*domain.AgentMessage, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, role, content, tool_calls_json, created_at FROM agent_messages WHERE id = ?`, id)
	var m domain.AgentMessage
	var content, toolCalls string
	var createdAt string
	if err := row.Scan(&m.ID, &m.SessionID, &m.Role, &content, &toolCalls, &createdAt); err != nil {
		return nil, err
	}
	m.Content = content
	m.ToolCallsJSON = toolCalls
	m.CreatedAt = parseSQLiteTime(createdAt)
	return &m, nil
}

func (r *Repository) ListAgentMessages(ctx context.Context, sessionID int64) ([]*domain.AgentMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, role, content, tool_calls_json, created_at FROM agent_messages
		WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.AgentMessage{}
	for rows.Next() {
		var m domain.AgentMessage
		var content, toolCalls string
		var createdAt string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &content, &toolCalls, &createdAt); err != nil {
			return nil, err
		}
		m.Content = content
		m.ToolCallsJSON = toolCalls
		m.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, &m)
	}
	return out, rows.Err()
}
