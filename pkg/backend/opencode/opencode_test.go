package opencode

import (
	"testing"
)

func TestParseLineText(t *testing.T) {
	line := []byte(`{"type":"text","sessionID":"ses_abc","part":{"messageID":"msg_1","text":"hello world"}}`)
	res, ok := parseLine(line, "anthropic/claude-sonnet-4-6")
	if !ok {
		t.Fatal("parseLine returned ok=false")
	}
	if res.SessionID != "ses_abc" {
		t.Errorf("SessionID = %q, want ses_abc", res.SessionID)
	}
	if !res.Emit {
		t.Error("Emit = false, want true")
	}
	if len(res.Event.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(res.Event.Content))
	}
	if res.Event.Content[0].Type != "text" || res.Event.Content[0].Text != "hello world" {
		t.Errorf("Content[0] = %+v, want {text, hello world}", res.Event.Content[0])
	}
	if res.Event.MessageID != "msg_1" {
		t.Errorf("MessageID = %q, want msg_1", res.Event.MessageID)
	}
	if res.Event.Model != "anthropic/claude-sonnet-4-6" {
		t.Errorf("Model = %q, want anthropic/claude-sonnet-4-6", res.Event.Model)
	}
}

func TestParseLineToolUseWithOutput(t *testing.T) {
	line := []byte(`{"type":"tool_use","sessionID":"ses_abc","part":{"messageID":"msg_2","tool":"Bash","state":{"status":"completed","input":{"command":"ls"},"output":"file.txt"}}}`)
	res, ok := parseLine(line, "m")
	if !ok {
		t.Fatal("parseLine returned ok=false")
	}
	if len(res.Event.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2 (tool_use + tool_result)", len(res.Event.Content))
	}
	if res.Event.Content[0].Type != "tool_use" || res.Event.Content[0].Name != "Bash" {
		t.Errorf("Content[0] = %+v, want {tool_use, Bash, ...}", res.Event.Content[0])
	}
	if res.Event.Content[0].Input != `{"command":"ls"}` {
		t.Errorf("Content[0].Input = %q, want {\"command\":\"ls\"}", res.Event.Content[0].Input)
	}
	if res.Event.Content[1].Type != "tool_result" || res.Event.Content[1].IsError {
		t.Errorf("Content[1] = %+v, want non-error tool_result", res.Event.Content[1])
	}
	if res.Event.Content[1].Content != "file.txt" {
		t.Errorf("Content[1].Content = %q, want file.txt", res.Event.Content[1].Content)
	}
}

func TestParseLineToolUseWithError(t *testing.T) {
	line := []byte(`{"type":"tool_use","sessionID":"ses_abc","part":{"messageID":"msg_3","tool":"Bash","state":{"status":"error","input":{"command":"bad"},"error":"command not found"}}}`)
	res, ok := parseLine(line, "m")
	if !ok {
		t.Fatal("parseLine returned ok=false")
	}
	if len(res.Event.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(res.Event.Content))
	}
	if !res.Event.Content[1].IsError {
		t.Error("Content[1].IsError = false, want true")
	}
	if res.Event.Content[1].Content != "command not found" {
		t.Errorf("Content[1].Content = %q, want command not found", res.Event.Content[1].Content)
	}
}

func TestParseLineStepFinish(t *testing.T) {
	line := []byte(`{"type":"step_finish","sessionID":"ses_abc","part":{"messageID":"msg_4","cost":0.0123,"tokens":{"input":100,"output":50,"cache":{"read":30,"write":20}}}}`)
	res, ok := parseLine(line, "m")
	if !ok {
		t.Fatal("parseLine returned ok=false")
	}
	if !res.StepTurn {
		t.Error("StepTurn = false, want true (step_finish should increment turns)")
	}
	if res.StepCost != 0.0123 {
		t.Errorf("StepCost = %v, want 0.0123", res.StepCost)
	}
	if len(res.Event.Content) != 0 {
		t.Errorf("len(Content) = %d, want 0 (step_finish has no content blocks)", len(res.Event.Content))
	}
	if res.Event.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", res.Event.InputTokens)
	}
	if res.Event.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", res.Event.OutputTokens)
	}
	if res.Event.CacheRead != 30 {
		t.Errorf("CacheRead = %d, want 30", res.Event.CacheRead)
	}
	if res.Event.CacheCreation != 20 {
		t.Errorf("CacheCreation = %d, want 20", res.Event.CacheCreation)
	}
}

func TestParseLineError(t *testing.T) {
	line := []byte(`{"type":"error","sessionID":"ses_abc","error":{"name":"APIError","data":{"message":"rate limit"}}}`)
	res, ok := parseLine(line, "m")
	if !ok {
		t.Fatal("parseLine returned ok=false")
	}
	if len(res.Event.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(res.Event.Content))
	}
	if !res.Event.Content[0].IsError {
		t.Error("Content[0].IsError = false, want true")
	}
	if res.Event.Content[0].Content != "rate limit" {
		t.Errorf("Content[0].Content = %q, want rate limit", res.Event.Content[0].Content)
	}
}

func TestParseLineErrorFallsBackToName(t *testing.T) {
	line := []byte(`{"type":"error","sessionID":"ses_abc","error":{"name":"APIError","data":{}}}`)
	res, ok := parseLine(line, "m")
	if !ok {
		t.Fatal("parseLine returned ok=false")
	}
	if res.Event.Content[0].Content != "APIError" {
		t.Errorf("Content[0].Content = %q, want APIError (fallback to name when no message)", res.Event.Content[0].Content)
	}
}

func TestParseLineMalformedJSON(t *testing.T) {
	line := []byte(`{"type":"text","part":{invalid`)
	_, ok := parseLine(line, "m")
	if ok {
		t.Error("parseLine returned ok=true for malformed JSON, want false")
	}
}

func TestParseLineUnknownType(t *testing.T) {
	line := []byte(`{"type":"session_started","sessionID":"ses_abc"}`)
	res, ok := parseLine(line, "m")
	if ok && res.Emit {
		t.Error("parseLine emitted event for unknown type")
	}
}
