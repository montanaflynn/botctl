package backend_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/montanaflynn/botctl/pkg/backend"
	_ "github.com/montanaflynn/botctl/pkg/backend/claude"
	_ "github.com/montanaflynn/botctl/pkg/backend/opencode"
)

func TestGetDefaultsToClaude(t *testing.T) {
	b, err := backend.Get("")
	if err != nil {
		t.Fatalf("Get(\"\") returned error: %v", err)
	}
	if b.Name() != "claude" {
		t.Fatalf("Get(\"\") returned %q, want %q", b.Name(), "claude")
	}
}

func TestGetOpencode(t *testing.T) {
	b, err := backend.Get("opencode")
	if err != nil {
		t.Fatalf("Get(\"opencode\") returned error: %v", err)
	}
	if b.Name() != "opencode" {
		t.Fatalf("Get(\"opencode\") returned %q, want %q", b.Name(), "opencode")
	}
}

func TestGetUnknownBackendListsAvailable(t *testing.T) {
	_, err := backend.Get("bogus")
	if err == nil {
		t.Fatal("Get(\"bogus\") returned nil error, expected one")
	}
	msg := err.Error()
	if !strings.Contains(msg, "claude") || !strings.Contains(msg, "opencode") {
		t.Fatalf("error %q should list registered backends (claude, opencode)", msg)
	}
}

func TestNamesSorted(t *testing.T) {
	names := backend.Names()
	if !slices.Contains(names, "claude") || !slices.Contains(names, "opencode") {
		t.Fatalf("Names() missing expected backends: %v", names)
	}
	if !slices.IsSorted(names) {
		t.Fatalf("Names() not sorted: %v", names)
	}
}
