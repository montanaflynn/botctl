package claude

import (
	"encoding/json"
	"strings"
)

// CanUseToolFunc is called when the CLI requests permission to use a tool.
// Return a PermissionResult to allow or deny the request. If nil is returned
// with no error, the tool use is auto-allowed.
type CanUseToolFunc func(toolName string, input map[string]any) (*PermissionResult, error)

// PermissionResult controls whether a tool use request is allowed or denied.
type PermissionResult struct {
	// Behavior is "allow" or "deny".
	Behavior string `json:"behavior"`

	// Message is an optional explanation (used with "deny").
	Message string `json:"message,omitempty"`

	// UpdatedInput optionally replaces the tool input when allowing.
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
}

// AgentDefinition configures a sub-agent passed in the initialize request.
type AgentDefinition struct {
	Description string   `json:"description,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Model       string   `json:"model,omitempty"`
}

// Options configures how the Claude CLI is invoked.
type Options struct {
	// SystemPrompt sets the system prompt for the conversation.
	SystemPrompt string

	// AppendSystemPrompt appends to (rather than replaces) the system prompt.
	AppendSystemPrompt string

	// AllowedTools restricts which tools Claude can use (e.g. "Bash", "Read", "Write").
	AllowedTools []string

	// Tools sets the base tool set (--tools flag).
	Tools []string

	// DisallowedTools blocks specific tools (--disallowedTools flag).
	DisallowedTools []string

	// Cwd sets the working directory for the Claude process.
	Cwd string

	// PermissionMode controls tool permission behavior.
	// Valid values: "default", "acceptEdits", "plan", "bypassPermissions".
	PermissionMode string

	// MaxTurns limits the number of agentic turns.
	MaxTurns int

	// MaxBudgetUSD sets a spending limit for the conversation.
	MaxBudgetUSD float64

	// Model overrides the default Claude model.
	Model string

	// FallbackModel is used when the primary model is unavailable.
	FallbackModel string

	// MaxBufferSize limits the context buffer (bytes).
	MaxBufferSize int

	// MaxThinkingTokens limits extended thinking tokens (--max-thinking-tokens).
	MaxThinkingTokens int

	// SessionID resumes a previous session.
	SessionID string

	// Continue continues the most recent conversation.
	Continue bool

	// ForkSession forks the session instead of continuing it (--fork-session).
	ForkSession bool

	// IncludePartialMessages includes partial/streaming messages (--include-partial-messages).
	IncludePartialMessages bool

	// CLIPath is the path to the claude binary. Defaults to "claude".
	CLIPath string

	// Settings is the path to a settings file (--settings).
	Settings string

	// SettingSources specifies which setting sources to use (--setting-sources, comma-separated).
	SettingSources []string

	// Betas enables beta features (--betas, comma-separated).
	Betas []string

	// AddDirs adds additional directories to the context (--add-dir, repeated).
	AddDirs []string

	// Env sets extra environment variables for the subprocess.
	Env map[string]string

	// ExtraArgs passes arbitrary CLI flags. Map keys are flag names (with dashes).
	// A nil value means a boolean flag (no argument). A non-nil value is the argument.
	ExtraArgs map[string]*string

	// OutputFormat configures structured output. When the map contains
	// "type": "json_schema", the schema is passed via --json-schema.
	OutputFormat map[string]any

	// MCPServers configures MCP server connections (--mcp-config).
	MCPServers map[string]any

	// Agents defines sub-agents sent in the initialize request.
	Agents map[string]AgentDefinition

	// EnvelopeHandler, if set, is called for each assistant envelope with full raw JSON.
	// When set, it replaces the MessageHandler passed to Query.
	EnvelopeHandler EnvelopeHandler

	// CanUseTool is called when the CLI requests tool permission.
	// If nil, tool use is auto-allowed.
	CanUseTool CanUseToolFunc

	// Stderr is called for each line of stderr output from the CLI process.
	Stderr func(string)

	// InterruptCh, if set, is checked between turns. When signaled, the query
	// stops at the next turn boundary and returns a result with Interrupted=true.
	InterruptCh <-chan struct{}
}

// cliPath returns the path to the claude binary, defaulting to "claude".
func (o *Options) cliPath() string {
	if o.CLIPath != "" {
		return o.CLIPath
	}
	return "claude"
}

// args builds the CLI arguments from Options.
func (o *Options) args() []string {
	var args []string

	if o.SystemPrompt != "" {
		args = append(args, "--system-prompt", o.SystemPrompt)
	}
	if o.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", o.AppendSystemPrompt)
	}
	if len(o.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(o.AllowedTools, ","))
	}
	if len(o.Tools) > 0 {
		args = append(args, "--tools", strings.Join(o.Tools, ","))
	}
	if len(o.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(o.DisallowedTools, ","))
	}
	if o.PermissionMode != "" {
		args = append(args, "--permission-mode", o.PermissionMode)
	}
	if o.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", ftoa(o.MaxBudgetUSD))
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.FallbackModel != "" {
		args = append(args, "--fallback-model", o.FallbackModel)
	}
	if o.MaxThinkingTokens > 0 {
		args = append(args, "--max-thinking-tokens", itoa(o.MaxThinkingTokens))
	}
	if o.SessionID != "" {
		args = append(args, "--resume", o.SessionID)
	}
	if o.Continue {
		args = append(args, "--continue")
	}
	if o.ForkSession {
		args = append(args, "--fork-session")
	}
	if o.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}
	if o.Settings != "" {
		args = append(args, "--settings", o.Settings)
	}
	if len(o.SettingSources) > 0 {
		args = append(args, "--setting-sources", strings.Join(o.SettingSources, ","))
	}
	if len(o.Betas) > 0 {
		args = append(args, "--betas", strings.Join(o.Betas, ","))
	}
	for _, dir := range o.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if o.MCPServers != nil {
		data, err := json.Marshal(o.MCPServers)
		if err == nil {
			args = append(args, "--mcp-config", string(data))
		}
	}
	if o.OutputFormat != nil {
		if typ, _ := o.OutputFormat["type"].(string); typ == "json_schema" {
			if schema, ok := o.OutputFormat["schema"]; ok {
				data, err := json.Marshal(schema)
				if err == nil {
					args = append(args, "--json-schema", string(data))
				}
			}
		}
	}

	// Extra args passthrough
	for k, v := range o.ExtraArgs {
		if v == nil {
			args = append(args, k)
		} else {
			args = append(args, k, *v)
		}
	}

	return args
}

// buildEnv constructs environment variable slice for the subprocess.
func (o *Options) buildEnv(base []string) []string {
	if len(o.Env) == 0 {
		return base
	}
	env := make([]string, len(base), len(base)+len(o.Env))
	copy(env, base)
	for k, v := range o.Env {
		env = append(env, k+"="+v)
	}
	return env
}

// agentsPayload returns the agents map for the initialize request, or nil.
func (o *Options) agentsPayload() map[string]any {
	if len(o.Agents) == 0 {
		return nil
	}
	m := make(map[string]any, len(o.Agents))
	for name, def := range o.Agents {
		m[name] = def
	}
	return m
}
