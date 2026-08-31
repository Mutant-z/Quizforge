package domain

import "time"

// AgentSession AI 侧栏会话
type AgentSession struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Title       string    `json:"title"`
	ContextJSON string    `json:"context_json,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentMessage 会话消息
type AgentMessage struct {
	ID            int64     `json:"id"`
	SessionID     int64     `json:"session_id"`
	Role          string    `json:"role"`
	Content       string    `json:"content,omitempty"`
	ToolCallsJSON string    `json:"tool_calls_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ToolCall 工具调用记录（响应给前端的状态事件）
type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
	Status    string `json:"status"` // running | completed | failed
	LatencyMs int64  `json:"latency_ms,omitempty"`
}
