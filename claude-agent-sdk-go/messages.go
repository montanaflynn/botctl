package claude

import "encoding/json"

// --- Message interface ---

// Message is implemented by all message types returned from the CLI.
type Message interface {
	messageType() string
}

// --- Content blocks within assistant messages ---

// ContentBlock represents a single block in an assistant message's content array.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
}

// IsText returns true if this is a text content block.
func (b *ContentBlock) IsText() bool { return b.Type == "text" }

// IsToolUse returns true if this is a tool_use content block.
func (b *ContentBlock) IsToolUse() bool { return b.Type == "tool_use" }

// IsToolResult returns true if this is a tool_result content block.
func (b *ContentBlock) IsToolResult() bool { return b.Type == "tool_result" }

// IsThinking returns true if this is a thinking content block.
func (b *ContentBlock) IsThinking() bool { return b.Type == "thinking" }

// ContentString returns the content as a string (for tool results).
func (b *ContentBlock) ContentString() string {
	if b.Content == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		return s
	}
	return string(b.Content)
}

// InputJSON returns the input as an indented JSON string (for tool use blocks).
func (b *ContentBlock) InputJSON() string {
	if b.Input == nil {
		return "{}"
	}
	out, err := json.MarshalIndent(json.RawMessage(b.Input), "", "  ")
	if err != nil {
		return string(b.Input)
	}
	return string(out)
}

// --- Messages from CLI stdout ---

// MessageEnvelope is the raw JSON envelope read from stdout.
// The Type field determines which concrete message it represents.
type MessageEnvelope struct {
	Type    string          `json:"type"`
	SubType string          `json:"subtype,omitempty"`
	Raw     json.RawMessage `json:"-"` // full raw JSON
}

// MessageUsage tracks per-message token consumption.
type MessageUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// AssistantMessage represents a "type":"assistant" message from Claude.
type AssistantMessage struct {
	ID         string         `json:"id,omitempty"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model,omitempty"`
	Usage      *MessageUsage  `json:"usage,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
}

func (AssistantMessage) messageType() string { return "assistant" }

// AssistantEnvelope wraps the outer envelope for assistant messages.
type AssistantEnvelope struct {
	Type            string           `json:"type"`
	Message         AssistantMessage `json:"message"`
	ParentToolUseID *string          `json:"parent_tool_use_id"`
	Error           *string          `json:"error"`
	SessionID       string           `json:"session_id,omitempty"`
	UUID            string           `json:"uuid,omitempty"`
	RawJSON         string           `json:"-"`
}

// ResultMessage represents a "type":"result" message — the final message in a conversation.
type ResultMessage struct {
	SubType          string `json:"subtype"`
	DurationMS       int    `json:"duration_ms"`
	DurationAPIMS    int    `json:"duration_api_ms"`
	IsError          bool   `json:"is_error"`
	NumTurns         int    `json:"num_turns"`
	SessionID        string `json:"session_id"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	Result           string  `json:"result"`
	Usage            *Usage  `json:"usage,omitempty"`
	StructuredOutput any     `json:"structured_output,omitempty"`
	Interrupted      bool   `json:"-"` // Set when query was stopped via InterruptCh
}

func (ResultMessage) messageType() string { return "result" }

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// UserMessage represents a "type":"user" message echoed back from the CLI.
type UserMessage struct {
	Type            string         `json:"type"`
	Message         UserContent    `json:"message"`
	UUID            string         `json:"uuid,omitempty"`
	ParentToolUseID *string        `json:"parent_tool_use_id,omitempty"`
	ToolUseResult   json.RawMessage `json:"tool_use_result,omitempty"`
}

func (UserMessage) messageType() string { return "user" }

// UserContent holds the content of a user message.
type UserContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SystemMessage represents a "type":"system" message from the CLI.
type SystemMessage struct {
	Type    string         `json:"type"`
	SubType string         `json:"subtype,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func (SystemMessage) messageType() string { return "system" }

// StreamEvent represents a "type":"stream_event" message from the CLI.
type StreamEvent struct {
	Type            string         `json:"type"`
	UUID            string         `json:"uuid,omitempty"`
	SessionID       string         `json:"session_id,omitempty"`
	Event           map[string]any `json:"event,omitempty"`
	ParentToolUseID *string        `json:"parent_tool_use_id,omitempty"`
}

func (StreamEvent) messageType() string { return "stream_event" }

// --- Control protocol ---

// controlRequest is a request from the CLI asking for permission or other actions.
type controlRequest struct {
	Type      string                `json:"type"`
	RequestID string                `json:"request_id"`
	Request   controlRequestPayload `json:"request"`
}

type controlRequestPayload struct {
	SubType  string          `json:"subtype"`
	ToolName string          `json:"tool_name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

// controlResponse is sent to the CLI via stdin to answer a control request.
type controlResponse struct {
	Type     string                 `json:"type"`
	Response controlResponsePayload `json:"response"`
}

type controlResponsePayload struct {
	SubType   string               `json:"subtype"`
	RequestID string               `json:"request_id"`
	Response  *controlResponseBody `json:"response,omitempty"`
}

type controlResponseBody struct {
	Behavior     string          `json:"behavior"`
	Message      string          `json:"message,omitempty"`
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`
}

// --- Messages sent to CLI stdin ---

// initializeRequest is the first message sent to the CLI.
type initializeRequest struct {
	Type      string                   `json:"type"`
	RequestID string                   `json:"request_id"`
	Request   initializeRequestPayload `json:"request"`
}

type initializeRequestPayload struct {
	SubType string            `json:"subtype"`
	Agents  map[string]any    `json:"agents,omitempty"`
}

// userInputMessage is sent to the CLI after initialization.
type userInputMessage struct {
	Type    string      `json:"type"`
	Message UserContent `json:"message"`
}

// --- Callback types for message handling ---

// MessageHandler is called for each assistant message received from Claude.
type MessageHandler func(msg AssistantMessage)

// EnvelopeHandler is called for each assistant envelope with full raw JSON.
type EnvelopeHandler func(env AssistantEnvelope)
