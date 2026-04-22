package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/montanaflynn/botctl/pkg/backend"
)

func init() {
	backend.Register(&openCodeBackend{})
}

type openCodeBackend struct{}

func (b *openCodeBackend) Name() string { return "opencode" }

func (b *openCodeBackend) Capabilities() backend.Capabilities {
	return backend.Capabilities{
		CostTracking: true,
	}
}

// JSONL event types from `opencode run --format json`.
//
// ⚠ Schema assumption: the `error` top-level field is confirmed from live
// output. The `part`-nested shapes for text/tool_use/step_finish were derived
// statically and should be validated against a real JSONL trace before trusting
// the fixture tests — see plan Phase 6.
type jsonlEvent struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp"`
	SessionID string          `json:"sessionID"`
	Part      json.RawMessage `json:"part"`
	Error     *errorEnvelope  `json:"error"`
}

type errorEnvelope struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type errorData struct {
	Message string `json:"message"`
}

type textPart struct {
	MessageID string `json:"messageID"`
	Text      string `json:"text"`
}

type toolUsePart struct {
	MessageID string       `json:"messageID"`
	Tool      string       `json:"tool"`
	State     toolUseState `json:"state"`
}

type toolUseState struct {
	Status string `json:"status"`
	Input  any    `json:"input"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

type stepFinishPart struct {
	MessageID string          `json:"messageID"`
	Cost      float64         `json:"cost"`
	Tokens    stepFinishToken `json:"tokens"`
}

type stepFinishToken struct {
	Input  int             `json:"input"`
	Output int             `json:"output"`
	Cache  stepFinishCache `json:"cache"`
}

type stepFinishCache struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

// parseResult carries what one JSONL line produced, so the scan loop can
// update accumulators (cost, turns) after delegating to parseLine.
type parseResult struct {
	Event     backend.MessageEvent
	Emit      bool    // true if Event should be passed to the handler
	SessionID string  // non-empty when the line carried a session id
	StepCost  float64 // non-zero only on step_finish
	StepTurn  bool    // true on step_finish — increments turn counter
}

// parseLine parses a single JSONL line into a parseResult. Returns ok=false
// if the line is not valid JSON or not a recognized event type. Pure function
// for unit testing; no I/O, no subprocess.
func parseLine(line []byte, modelName string) (parseResult, bool) {
	var event jsonlEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return parseResult{}, false
	}

	res := parseResult{SessionID: event.SessionID}

	switch event.Type {
	case "text":
		var part textPart
		if err := json.Unmarshal(event.Part, &part); err != nil {
			return parseResult{}, false
		}
		res.Event = backend.MessageEvent{
			Content: []backend.ContentBlock{{
				Type: "text",
				Text: part.Text,
			}},
			MessageID: part.MessageID,
			Model:     modelName,
		}
		res.Emit = true

	case "tool_use":
		var part toolUsePart
		if err := json.Unmarshal(event.Part, &part); err != nil {
			return parseResult{}, false
		}
		inputJSON := "{}"
		if part.State.Input != nil {
			if data, err := json.Marshal(part.State.Input); err == nil {
				inputJSON = string(data)
			}
		}
		blocks := []backend.ContentBlock{{
			Type:  "tool_use",
			Name:  part.Tool,
			Input: inputJSON,
		}}
		if part.State.Output != "" {
			blocks = append(blocks, backend.ContentBlock{
				Type:    "tool_result",
				Content: part.State.Output,
			})
		}
		if part.State.Error != "" {
			blocks = append(blocks, backend.ContentBlock{
				Type:    "tool_result",
				Content: part.State.Error,
				IsError: true,
			})
		}
		res.Event = backend.MessageEvent{
			Content:   blocks,
			MessageID: part.MessageID,
			Model:     modelName,
		}
		res.Emit = true

	case "step_finish":
		var part stepFinishPart
		if err := json.Unmarshal(event.Part, &part); err != nil {
			return parseResult{}, false
		}
		res.Event = backend.MessageEvent{
			MessageID:     part.MessageID,
			Model:         modelName,
			InputTokens:   part.Tokens.Input,
			OutputTokens:  part.Tokens.Output,
			CacheRead:     part.Tokens.Cache.Read,
			CacheCreation: part.Tokens.Cache.Write,
		}
		res.Emit = true
		res.StepCost = part.Cost
		res.StepTurn = true

	case "error":
		var msg string
		if event.Error != nil {
			var data errorData
			_ = json.Unmarshal(event.Error.Data, &data)
			msg = data.Message
			if msg == "" {
				msg = event.Error.Name
			}
		}
		if msg == "" {
			return parseResult{}, false
		}
		res.Event = backend.MessageEvent{
			Content: []backend.ContentBlock{{
				Type:    "tool_result",
				Content: msg,
				IsError: true,
			}},
			Model: modelName,
		}
		res.Emit = true

	default:
		return parseResult{}, false
	}

	return res, true
}

func (b *openCodeBackend) Query(ctx context.Context, prompt string, opts backend.Options, handler backend.MessageHandler) (*backend.Result, error) {
	// System prompt injection: opencode has no --system-prompt flag, so prepend
	// to the user prompt with a clear delimiter.
	effectivePrompt := prompt
	if opts.SystemPrompt != "" {
		effectivePrompt = fmt.Sprintf(
			"# System instructions (follow these before responding)\n\n%s\n\n# User message\n\n%s",
			opts.SystemPrompt, prompt,
		)
	}

	modelName := opts.Model
	if opts.Provider != "" && opts.Model != "" {
		modelName = opts.Provider + "/" + opts.Model
	}

	args := []string{"run", "--format", "json", "--dangerously-skip-permissions"}

	if modelName != "" {
		args = append(args, "--model", modelName)
	}

	if opts.SessionID != "" {
		args = append(args, "--session", opts.SessionID)
	}

	args = append(args, effectivePrompt)

	cmd := exec.CommandContext(ctx, "opencode", args...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	cmd.Env = os.Environ()
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start opencode: %w", err)
	}

	// Interrupt forwarder: watches InterruptCh concurrently so signals fire
	// even when opencode is silent on stdout for long stretches.
	stop := make(chan struct{})
	interrupted := false
	if opts.InterruptCh != nil {
		go func() {
			select {
			case <-opts.InterruptCh:
				interrupted = true
				if cmd.Process != nil {
					_ = cmd.Process.Signal(os.Interrupt)
				}
			case <-stop:
			}
		}()
	}

	startTime := time.Now()
	var sessionID string
	var totalCost float64
	var turns int

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 10*1024*1024), 10*1024*1024)

	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		res, ok := parseLine(lineBytes, modelName)
		if !ok {
			continue
		}
		if res.SessionID != "" {
			sessionID = res.SessionID
		}
		if res.Emit {
			res.Event.RawJSON = string(lineBytes)
			handler(res.Event)
		}
		if res.StepTurn {
			turns++
			totalCost += res.StepCost
			if opts.MaxTurns > 0 && turns >= opts.MaxTurns {
				interrupted = true
				if cmd.Process != nil {
					_ = cmd.Process.Signal(os.Interrupt)
				}
				break
			}
		}
	}

	close(stop)
	waitErr := cmd.Wait()

	durationMS := int(time.Since(startTime).Milliseconds())
	result := &backend.Result{
		SessionID:    sessionID,
		NumTurns:     turns,
		TotalCostUSD: totalCost,
		DurationMS:   durationMS,
		Interrupted:  interrupted,
	}

	if waitErr != nil && !interrupted {
		return result, fmt.Errorf("opencode exited: %w: %s", waitErr, stderr.String())
	}
	return result, nil
}
