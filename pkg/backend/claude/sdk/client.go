package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// ftoa converts float64 to string.
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// itoa converts int to string.
func itoa(i int) string { return strconv.Itoa(i) }

// Query spawns the Claude CLI, sends a prompt, streams messages to the handler,
// and returns the final ResultMessage. It respects context cancellation.
//
// The handler receives each assistant message as it arrives. If opts.EnvelopeHandler
// is set, it takes priority over handler.
func Query(ctx context.Context, prompt string, opts Options, handler MessageHandler) (*ResultMessage, error) {
	transport := NewSubprocessTransport(opts)

	if err := transport.Connect(ctx); err != nil {
		return nil, err
	}
	defer transport.Close()

	// Send initialize request
	initReq := initializeRequest{
		Type:      "control_request",
		RequestID: "init_1",
		Request: initializeRequestPayload{
			SubType: "initialize",
			Agents:  opts.agentsPayload(),
		},
	}
	if err := transport.WriteJSON(initReq); err != nil {
		_ = transport.Wait()
		return nil, &CLIConnectionError{SDKError: SDKError{Message: "write initialize", Err: err}}
	}

	// Send user message
	userMsg := userInputMessage{
		Type: "user",
		Message: UserContent{
			Role:    "user",
			Content: prompt,
		},
	}
	if err := transport.WriteJSON(userMsg); err != nil {
		_ = transport.Wait()
		return nil, &CLIConnectionError{SDKError: SDKError{Message: "write user message", Err: err}}
	}

	var result *ResultMessage
	var readErr error
	var turnCount int
	var lastSessionID string
	var hitMaxTurns bool
	var interrupted bool

	// When resuming a session, the CLI replays prior messages before processing
	// the new user message. We must not count replayed assistant messages toward
	// MaxTurns — only count turns after we see our new user message echoed back.
	countingTurns := opts.SessionID == ""

scanLoop:
	for raw := range transport.ReadMessages() {
		if raw.Err != nil {
			readErr = raw.Err
			break
		}

		line := raw.Data

		// Peek at the type field
		var envelope MessageEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "user":
			// The CLI echoes the user message after replay is done.
			countingTurns = true

		case "assistant":
			var env AssistantEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				continue
			}
			env.RawJSON = string(line)
			if env.SessionID != "" {
				lastSessionID = env.SessionID
			}
			if opts.EnvelopeHandler != nil {
				opts.EnvelopeHandler(env)
			} else if handler != nil {
				handler(env.Message)
			}
			if countingTurns {
				turnCount++
			}
			if countingTurns && opts.MaxTurns > 0 && turnCount >= opts.MaxTurns {
				hitMaxTurns = true
				transport.Close()
				break scanLoop
			}
			if countingTurns && opts.InterruptCh != nil {
				select {
				case <-opts.InterruptCh:
					interrupted = true
					transport.Close()
					break scanLoop
				default:
				}
			}

		case "result":
			var r ResultMessage
			if err := json.Unmarshal(line, &r); err != nil {
				continue
			}
			result = &r
			_ = transport.EndInput()

		case "control_request":
			var req controlRequest
			if err := json.Unmarshal(line, &req); err != nil {
				continue
			}

			if req.Request.SubType == "can_use_tool" {
				resp, err := handleToolPermission(opts, req, transport)
				if err != nil {
					readErr = err
				}
				_ = resp // response already sent via transport
			}

		case "control_response":
			// Response to our initialize — ignore

		default:
			// system, stream_event — ignore
		}
	}

	// Wait for process
	_ = transport.EndInput()
	waitErr := transport.Wait()

	// If we hit max turns or were interrupted, synthesize a result with session ID
	if (hitMaxTurns || interrupted) && result == nil {
		return &ResultMessage{
			SessionID:   lastSessionID,
			NumTurns:    turnCount,
			Interrupted: interrupted,
		}, nil
	}

	if readErr != nil {
		return result, readErr
	}

	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	if waitErr != nil && result == nil {
		if (opts.MaxTurns > 0 || interrupted) && turnCount > 0 {
			return &ResultMessage{
				SessionID:   lastSessionID,
				NumTurns:    turnCount,
				Interrupted: interrupted,
			}, nil
		}
		return nil, toProcessError(waitErr)
	}

	return result, nil
}

// handleToolPermission processes a can_use_tool control request.
func handleToolPermission(opts Options, req controlRequest, transport Transport) (bool, error) {
	var resp controlResponse

	if opts.CanUseTool != nil {
		// Parse input
		var input map[string]any
		if req.Request.Input != nil {
			_ = json.Unmarshal(req.Request.Input, &input)
		}

		perm, err := opts.CanUseTool(req.Request.ToolName, input)
		if err != nil {
			// On error, deny the tool use
			resp = controlResponse{
				Type: "control_response",
				Response: controlResponsePayload{
					SubType:   "success",
					RequestID: req.RequestID,
					Response: &controlResponseBody{
						Behavior: "deny",
						Message:  err.Error(),
					},
				},
			}
		} else if perm != nil {
			body := &controlResponseBody{
				Behavior: perm.Behavior,
				Message:  perm.Message,
			}
			if perm.UpdatedInput != nil {
				data, _ := json.Marshal(perm.UpdatedInput)
				body.UpdatedInput = data
			}
			resp = controlResponse{
				Type: "control_response",
				Response: controlResponsePayload{
					SubType:   "success",
					RequestID: req.RequestID,
					Response:  body,
				},
			}
		} else {
			// nil result means auto-allow
			resp = controlResponse{
				Type: "control_response",
				Response: controlResponsePayload{
					SubType:   "success",
					RequestID: req.RequestID,
					Response:  &controlResponseBody{Behavior: "allow"},
				},
			}
		}
	} else {
		// Auto-allow when no callback is set
		resp = controlResponse{
			Type: "control_response",
			Response: controlResponsePayload{
				SubType:   "success",
				RequestID: req.RequestID,
				Response:  &controlResponseBody{Behavior: "allow"},
			},
		}
	}

	if err := transport.WriteJSON(resp); err != nil {
		return false, fmt.Errorf("write control response: %w", err)
	}
	return true, nil
}

// toProcessError converts a cmd.Wait() error into a ProcessError when possible.
func toProcessError(err error) error {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return &ProcessError{
			SDKError: SDKError{Message: "claude exited", Err: err},
			ExitCode: exitErr.ExitCode(),
			Stderr:   string(exitErr.Stderr),
		}
	}
	return &ProcessError{
		SDKError: SDKError{Message: "claude exited", Err: err},
		ExitCode: -1,
	}
}
