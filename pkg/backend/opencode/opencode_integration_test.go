//go:build integration

package opencode

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/montanaflynn/botctl/pkg/backend"
)

// TestIntegrationOpencodeQuery runs a real `opencode run` subprocess against
// whatever provider is configured. Skipped unless the opencode binary is on
// PATH and OPENCODE_TEST_MODEL is set (e.g. "openrouter/openai/gpt-oss-120b").
//
// Run with: `go test -tags=integration ./pkg/backend/opencode/`
func TestIntegrationOpencodeQuery(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode binary not on PATH")
	}
	model := os.Getenv("OPENCODE_TEST_MODEL")
	if model == "" {
		t.Skip("OPENCODE_TEST_MODEL not set (e.g. openrouter/openai/gpt-oss-120b)")
	}

	// Split model into provider/model for the backend Options.
	var provider, modelID string
	if i := strings.Index(model, "/"); i > 0 {
		provider = model[:i]
		modelID = model[i+1:]
	} else {
		modelID = model
	}

	b := &openCodeBackend{}
	var sawText bool

	result, err := b.Query(
		context.Background(),
		"Reply with exactly the word PINEAPPLE and nothing else.",
		backend.Options{
			SystemPrompt: "You follow instructions precisely.",
			Provider:     provider,
			Model:        modelID,
			MaxTurns:     1,
		},
		func(ev backend.MessageEvent) {
			for _, block := range ev.Content {
				if block.Type == "text" && strings.Contains(strings.ToUpper(block.Text), "PINEAPPLE") {
					sawText = true
				}
			}
		},
	)

	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if result == nil {
		t.Fatal("Query returned nil result")
	}
	if result.SessionID == "" {
		t.Error("Result.SessionID is empty")
	}
	if result.NumTurns == 0 {
		t.Error("Result.NumTurns is 0")
	}
	if !sawText {
		t.Error("did not see PINEAPPLE shibboleth in any text block — system prompt injection may not be reaching the model")
	}
}
