package claude

import (
	"context"

	"github.com/montanaflynn/botctl/pkg/backend"
	sdk "github.com/montanaflynn/claude-agent-sdk-go"
)

func init() {
	backend.Register(&claudeBackend{})
}

type claudeBackend struct{}

func (b *claudeBackend) Name() string { return "claude" }

func (b *claudeBackend) Capabilities() backend.Capabilities {
	return backend.Capabilities{
		CostTracking: true,
	}
}

func (b *claudeBackend) Query(ctx context.Context, prompt string, opts backend.Options, handler backend.MessageHandler) (*backend.Result, error) {
	sdkOpts := sdk.Options{
		SystemPrompt:   opts.SystemPrompt,
		Cwd:            opts.Cwd,
		PermissionMode: "bypassPermissions",
		MaxBufferSize:  10 * 1024 * 1024,
		InterruptCh:    opts.InterruptCh,
	}
	if opts.MaxTurns > 0 {
		sdkOpts.MaxTurns = opts.MaxTurns
	}
	if opts.SessionID != "" {
		sdkOpts.SessionID = opts.SessionID
	}
	if opts.Env != nil {
		sdkOpts.Env = opts.Env
	}

	sdkOpts.EnvelopeHandler = func(env sdk.AssistantEnvelope) {
		msg := env.Message
		event := backend.MessageEvent{
			MessageID: msg.ID,
			Model:     msg.Model,
			RawJSON:   env.RawJSON,
		}
		if msg.Usage != nil {
			event.InputTokens = msg.Usage.InputTokens
			event.OutputTokens = msg.Usage.OutputTokens
			event.CacheCreation = msg.Usage.CacheCreationInputTokens
			event.CacheRead = msg.Usage.CacheReadInputTokens
		}
		for _, block := range msg.Content {
			cb := backend.ContentBlock{Type: block.Type}
			switch {
			case block.IsText():
				cb.Text = block.Text
			case block.IsToolUse():
				cb.Name = block.Name
				cb.Input = block.InputJSON()
			case block.IsToolResult():
				cb.Content = block.ContentString()
				cb.IsError = block.IsError
			}
			event.Content = append(event.Content, cb)
		}
		handler(event)
	}

	result, err := sdk.Query(ctx, prompt, sdkOpts, nil)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &backend.Result{
		SessionID:    result.SessionID,
		NumTurns:     result.NumTurns,
		TotalCostUSD: result.TotalCostUSD,
		DurationMS:   result.DurationMS,
		Interrupted:  result.Interrupted,
	}, nil
}
