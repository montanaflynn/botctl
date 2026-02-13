package create

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/montanaflynn/botctl/pkg/paths"
	claude "github.com/montanaflynn/claude-agent-sdk-go"
)

//go:embed create_prompt.md
var createPrompt string

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ValidName reports whether name is a valid bot name.
func ValidName(name string) bool {
	return validNameRe.MatchString(name)
}

// Params holds the inputs for bot creation.
type Params struct {
	Name        string
	Description string
	Interval    int
	MaxTurns    int
}

// InteractiveParams prompts the user via stdin for creation parameters.
func InteractiveParams(p Params) (Params, error) {
	scanner := bufio.NewScanner(os.Stdin)

	if p.Name == "" {
		fmt.Print("Bot name: ")
		if !scanner.Scan() {
			return p, fmt.Errorf("no input")
		}
		p.Name = strings.TrimSpace(scanner.Text())
		if p.Name == "" {
			return p, fmt.Errorf("name is required")
		}
	}

	if p.Description == "" {
		fmt.Print("Description: ")
		if !scanner.Scan() {
			return p, fmt.Errorf("no input")
		}
		p.Description = strings.TrimSpace(scanner.Text())
		if p.Description == "" {
			return p, fmt.Errorf("description is required")
		}
	}

	if p.Interval == 0 {
		fmt.Print("Interval seconds [300]: ")
		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				n, err := strconv.Atoi(text)
				if err != nil || n < 1 {
					return p, fmt.Errorf("invalid interval: %s", text)
				}
				p.Interval = n
			}
		}
		if p.Interval == 0 {
			p.Interval = 300
		}
	}

	if p.MaxTurns == 0 {
		fmt.Print("Max turns [20]: ")
		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				n, err := strconv.Atoi(text)
				if err != nil || n < 1 {
					return p, fmt.Errorf("invalid max turns: %s", text)
				}
				p.MaxTurns = n
			}
		}
		if p.MaxTurns == 0 {
			p.MaxTurns = 20
		}
	}

	return p, nil
}

// cleanup removes the bot directory if it's empty.
func cleanup(botDir string) {
	os.Remove(botDir)
}

// formatToolUse returns a short description of a tool_use content block.
func formatToolUse(block claude.ContentBlock) string {
	var input map[string]any
	if block.Input != nil {
		_ = json.Unmarshal(block.Input, &input)
	}
	switch block.Name {
	case "Write":
		if p, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("Write %s", p)
		}
	case "Read":
		if p, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("Read %s", p)
		}
	}
	return block.Name
}

// Run generates a BOT.md via Claude and writes it to the bots directory.
// If progress is non-nil, status lines are sent as Claude works.
// The channel is never closed by Run.
func Run(p Params, progress chan<- string) (string, error) {
	if !ValidName(p.Name) {
		return "", fmt.Errorf("invalid bot name %q: use alphanumeric characters, hyphens, and underscores", p.Name)
	}

	send := func(s string) {
		if progress != nil {
			progress <- s
		}
	}

	botDir := filepath.Join(paths.BotsDir(), p.Name)
	botFile := filepath.Join(botDir, "BOT.md")
	if _, err := os.Stat(botFile); err == nil {
		return "", fmt.Errorf("bot %q already exists at %s", p.Name, botDir)
	}

	if err := os.MkdirAll(botDir, 0o755); err != nil {
		return "", fmt.Errorf("create bot directory: %w", err)
	}

	send("Calling Claude...")

	prompt := fmt.Sprintf(
		"Create a bot named %q. Description: %s. Use interval_seconds: %d, max_turns: %d.\n\n"+
			"The bots directory is: %s\n"+
			"Write the BOT.md file to: %s",
		p.Name, p.Description, p.Interval, p.MaxTurns,
		paths.BotsDir(), botFile,
	)

	var stderrLines []string
	result, err := claude.Query(context.Background(), prompt, claude.Options{
		SystemPrompt:   createPrompt,
		AllowedTools:   []string{"Write"},
		Cwd:            botDir,
		MaxTurns:       5,
		PermissionMode: "bypassPermissions",
		Stderr: func(line string) {
			stderrLines = append(stderrLines, line)
		},
	}, func(msg claude.AssistantMessage) {
		for _, block := range msg.Content {
			switch {
			case block.IsText() && block.Text != "":
				send(block.Text)
			case block.IsToolUse():
				send(formatToolUse(block))
			}
		}
	})
	if err != nil {
		cleanup(botDir)
		if len(stderrLines) > 0 {
			return "", fmt.Errorf("claude: %w\nstderr: %s", err, strings.Join(stderrLines, "\n"))
		}
		return "", fmt.Errorf("claude: %w", err)
	}

	if result.IsError {
		cleanup(botDir)
		detail := result.Result
		if detail == "" && len(stderrLines) > 0 {
			detail = strings.Join(stderrLines, "\n")
		}
		if detail != "" {
			return "", fmt.Errorf("claude error: %s", detail)
		}
		return "", fmt.Errorf("claude returned an error (no details)")
	}

	if _, err := os.Stat(botFile); err != nil {
		cleanup(botDir)
		return "", fmt.Errorf("claude did not create BOT.md (result: %s)", result.Result)
	}

	return botFile, nil
}
