package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// Client provides a bidirectional, multi-turn interface to the Claude CLI.
// Unlike Query(), which handles a single prompt-response cycle, Client
// allows sending multiple messages and receiving streaming responses.
type Client struct {
	opts      Options
	transport Transport

	mu         sync.Mutex
	connected  bool
	serverInfo map[string]any

	// Control request routing
	reqCounter atomic.Int64
	pending    sync.Map // requestID -> chan json.RawMessage

	// Message dispatch
	messages chan Message
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewClient creates a new Client with the given options.
// Call Connect to start the CLI process.
func NewClient(opts Options) *Client {
	return &Client{
		opts:     opts,
		messages: make(chan Message, 64),
		done:     make(chan struct{}),
	}
}

// Connect starts the CLI process and initializes the session.
// The prompt is sent as the first user message.
func (c *Client) Connect(ctx context.Context, prompt string) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return &SDKError{Message: "already connected"}
	}
	c.mu.Unlock()

	c.ctx, c.cancel = context.WithCancel(ctx)

	c.transport = NewSubprocessTransport(c.opts)
	if err := c.transport.Connect(c.ctx); err != nil {
		return err
	}

	// Send initialize
	initReq := initializeRequest{
		Type:      "control_request",
		RequestID: "init_1",
		Request: initializeRequestPayload{
			SubType: "initialize",
			Agents:  c.opts.agentsPayload(),
		},
	}
	if err := c.transport.WriteJSON(initReq); err != nil {
		c.transport.Close()
		return &CLIConnectionError{SDKError: SDKError{Message: "write initialize", Err: err}}
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	// Start reading messages
	go c.readLoop()

	// Send the initial prompt
	if prompt != "" {
		if err := c.sendUserMessage(prompt); err != nil {
			c.Disconnect()
			return err
		}
	}

	return nil
}

// Query sends a user message and returns after the response is complete.
// This is a convenience wrapper that collects all messages from one response cycle.
func (c *Client) Query(ctx context.Context, prompt string) error {
	return c.sendUserMessage(prompt)
}

// ReceiveMessages returns a channel that yields all messages from the CLI.
// The channel is closed when the client disconnects or the context is cancelled.
func (c *Client) ReceiveMessages(ctx context.Context) <-chan Message {
	out := make(chan Message, 64)
	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-c.messages:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			case <-c.done:
				return
			}
		}
	}()
	return out
}

// ReceiveResponse returns a channel that yields messages until a ResultMessage is received.
// After the ResultMessage, the channel is closed.
func (c *Client) ReceiveResponse(ctx context.Context) <-chan Message {
	out := make(chan Message, 64)
	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-c.messages:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
				if _, isResult := msg.(ResultMessage); isResult {
					return
				}
			case <-ctx.Done():
				return
			case <-c.done:
				return
			}
		}
	}()
	return out
}

// Interrupt sends an interrupt signal to pause Claude's current response.
func (c *Client) Interrupt(ctx context.Context) error {
	return c.sendControlRequest(ctx, "interrupt", nil)
}

// SetPermissionMode changes the permission mode for the current session.
func (c *Client) SetPermissionMode(ctx context.Context, mode string) error {
	return c.sendControlRequest(ctx, "set_permission_mode", map[string]any{
		"permission_mode": mode,
	})
}

// SetModel changes the model for subsequent turns.
func (c *Client) SetModel(ctx context.Context, model string) error {
	return c.sendControlRequest(ctx, "set_model", map[string]any{
		"model": model,
	})
}

// GetMCPStatus requests the current MCP server status.
func (c *Client) GetMCPStatus(ctx context.Context) (map[string]any, error) {
	respData, err := c.sendControlRequestWithResponse(ctx, "get_mcp_status", nil)
	if err != nil {
		return nil, err
	}
	var status map[string]any
	if err := json.Unmarshal(respData, &status); err != nil {
		return nil, &JSONDecodeError{SDKError: SDKError{Err: err}, Line: string(respData)}
	}
	return status, nil
}

// GetServerInfo returns the server info received during initialization.
func (c *Client) GetServerInfo() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverInfo
}

// Disconnect stops the CLI process and closes the transport.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}
	c.connected = false

	if c.cancel != nil {
		c.cancel()
	}

	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

// Close is an alias for Disconnect.
func (c *Client) Close() error {
	return c.Disconnect()
}

// sendUserMessage sends a user message to the CLI.
func (c *Client) sendUserMessage(content string) error {
	msg := userInputMessage{
		Type: "user",
		Message: UserContent{
			Role:    "user",
			Content: content,
		},
	}
	return c.transport.WriteJSON(msg)
}

// sendControlRequest sends a control request and doesn't wait for a response.
func (c *Client) sendControlRequest(ctx context.Context, subType string, data map[string]any) error {
	reqID := fmt.Sprintf("req_%d", c.reqCounter.Add(1))

	type controlReq struct {
		Type      string         `json:"type"`
		RequestID string         `json:"request_id"`
		Request   map[string]any `json:"request"`
	}

	reqPayload := map[string]any{
		"subtype": subType,
	}
	for k, v := range data {
		reqPayload[k] = v
	}

	req := controlReq{
		Type:      "control_request",
		RequestID: reqID,
		Request:   reqPayload,
	}

	return c.transport.WriteJSON(req)
}

// sendControlRequestWithResponse sends a control request and waits for the response.
func (c *Client) sendControlRequestWithResponse(ctx context.Context, subType string, data map[string]any) (json.RawMessage, error) {
	reqID := fmt.Sprintf("req_%d", c.reqCounter.Add(1))

	respCh := make(chan json.RawMessage, 1)
	c.pending.Store(reqID, respCh)
	defer c.pending.Delete(reqID)

	type controlReq struct {
		Type      string         `json:"type"`
		RequestID string         `json:"request_id"`
		Request   map[string]any `json:"request"`
	}

	reqPayload := map[string]any{
		"subtype": subType,
	}
	for k, v := range data {
		reqPayload[k] = v
	}

	req := controlReq{
		Type:      "control_request",
		RequestID: reqID,
		Request:   reqPayload,
	}

	if err := c.transport.WriteJSON(req); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, &SDKError{Message: "client disconnected"}
	}
}

// readLoop reads messages from the transport and dispatches them.
func (c *Client) readLoop() {
	defer close(c.done)
	defer close(c.messages)

	for raw := range c.transport.ReadMessages() {
		if raw.Err != nil {
			break
		}

		line := raw.Data

		var envelope MessageEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "user":
			var msg UserMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			c.emit(msg)

		case "assistant":
			var env AssistantEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				continue
			}
			env.RawJSON = string(line)

			if c.opts.EnvelopeHandler != nil {
				c.opts.EnvelopeHandler(env)
			}
			c.emit(env.Message)

		case "result":
			var r ResultMessage
			if err := json.Unmarshal(line, &r); err != nil {
				continue
			}
			c.emit(r)

		case "system":
			var msg SystemMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			c.emit(msg)

		case "stream_event":
			var evt StreamEvent
			if err := json.Unmarshal(line, &evt); err != nil {
				continue
			}
			c.emit(evt)

		case "control_request":
			var req controlRequest
			if err := json.Unmarshal(line, &req); err != nil {
				continue
			}
			if req.Request.SubType == "can_use_tool" {
				_, _ = handleToolPermission(c.opts, req, c.transport)
			}

		case "control_response":
			// Route to pending request channels
			var resp struct {
				Response struct {
					RequestID string          `json:"request_id"`
					Response  json.RawMessage `json:"response"`
				} `json:"response"`
			}
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			if ch, ok := c.pending.LoadAndDelete(resp.Response.RequestID); ok {
				respCh := ch.(chan json.RawMessage)
				select {
				case respCh <- resp.Response.Response:
				default:
				}
			}

			// Capture server info from init response
			if resp.Response.Response != nil {
				var info map[string]any
				if err := json.Unmarshal(resp.Response.Response, &info); err == nil {
					c.mu.Lock()
					if c.serverInfo == nil {
						c.serverInfo = info
					}
					c.mu.Unlock()
				}
			}
		}
	}
}

// emit sends a message to the messages channel without blocking.
func (c *Client) emit(msg Message) {
	select {
	case c.messages <- msg:
	default:
		// Drop if channel is full — prevents deadlock
	}
}
