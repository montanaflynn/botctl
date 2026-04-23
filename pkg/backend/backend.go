package backend

import "context"

// ContentBlock represents a unit of content from the agent.
type ContentBlock struct {
	Type    string // "text", "tool_use", "tool_result"
	Text    string // for text blocks
	Name    string // for tool_use blocks (tool name)
	Input   string // for tool_use blocks (JSON input)
	Content string // for tool_result blocks
	IsError bool   // for tool_result blocks
}

// MessageEvent is delivered for each assistant message during execution.
type MessageEvent struct {
	Content      []ContentBlock
	MessageID    string
	Model        string
	RawJSON      string
	InputTokens  int
	OutputTokens int
	CacheCreation int
	CacheRead     int
}

// Result is returned when execution completes.
type Result struct {
	SessionID    string
	NumTurns     int
	TotalCostUSD float64
	DurationMS   int
	Interrupted  bool
}

// Options configures a single agent execution.
type Options struct {
	SystemPrompt string
	Cwd          string
	MaxTurns     int
	SessionID    string
	InterruptCh  <-chan struct{}
	Env          map[string]string
	Model        string // model ID (e.g. "claude-sonnet-4-20250514")
	Provider     string // provider ID (e.g. "anthropic", "openai")
}

// MessageHandler receives streaming messages during execution.
type MessageHandler func(event MessageEvent)

// Backend is the interface that all agent backends must implement.
type Backend interface {
	// Name returns the backend identifier (e.g. "claude", "opencode", "goose").
	Name() string

	// Query executes a prompt and streams results to the handler.
	Query(ctx context.Context, prompt string, opts Options, handler MessageHandler) (*Result, error)

	// Capabilities reports which optional features this backend supports.
	Capabilities() Capabilities
}

// Capabilities declares what a backend can do.
type Capabilities struct {
	CostTracking bool
}
