package logs

import (
	"strings"

	"github.com/montanaflynn/botctl-go/internal/db"
)

// RenderEntry converts a LogEntry into display lines.
func RenderEntry(e db.LogEntry) []string {
	switch e.Kind {
	case "run_header":
		// heading is like "Run #1"
		num := strings.TrimPrefix(e.Heading, "Run #")
		return []string{"<run number=" + num + ">"}
	case "feedback":
		lines := []string{"<message>"}
		if e.Body != "" {
			lines = append(lines, e.Body)
		}
		return lines
	case "text":
		if e.Body == "" {
			return nil
		}
		return strings.Split(e.Body, "\n")
	case "tool_use":
		line := renderToolUseTag(e.Heading, e.Body)
		lines := []string{line}
		if e.Body != "" {
			// For tools where body is consumed into the tag attribute, don't repeat it
			if !toolBodyInTag(e.Heading) {
				lines = append(lines, strings.Split(e.Body, "\n")...)
			}
		}
		return lines
	case "tool_result":
		lines := []string{"<result>"}
		if e.Body != "" {
			lines = append(lines, strings.Split(e.Body, "\n")...)
		}
		return lines
	case "tool_error":
		lines := []string{"<error>"}
		if e.Body != "" {
			lines = append(lines, strings.Split(e.Body, "\n")...)
		}
		return lines
	case "cost":
		// body is like "$0.0763 | 3 turns | 16.7s"
		tag := "<cost"
		if e.Body != "" {
			parts := strings.Split(e.Body, " | ")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				switch {
				case strings.HasPrefix(p, "$"):
					tag += " usd=" + p
				case strings.HasSuffix(p, "turns"):
					tag += " turns=" + strings.TrimSuffix(p, " turns")
				case strings.HasSuffix(p, "s"):
					tag += " time=" + p
				}
			}
		}
		tag += ">"
		return []string{tag}
	case "max_turns":
		// heading is like "Max turns reached (30/30)"
		tag := "<max_turns"
		if i := strings.Index(e.Heading, "("); i >= 0 {
			if j := strings.Index(e.Heading[i:], ")"); j >= 0 {
				tag += " reached=\"" + e.Heading[i+1:i+j] + "\""
			}
		}
		tag += ">"
		lines := []string{tag}
		if e.Body != "" {
			lines = append(lines, e.Body)
		}
		return lines
	case "sleep":
		// body is like "sleeping 60s..."
		tag := "<sleep"
		if e.Body != "" {
			s := strings.TrimPrefix(e.Body, "sleeping ")
			s = strings.TrimSuffix(s, "...")
			if s != "" {
				tag += " time=" + s
			}
		}
		tag += ">"
		return []string{tag}
	case "resume":
		return []string{"<resume>"}
	case "event":
		if e.Heading != "" {
			return []string{"<event type=\"" + e.Heading + "\">"}
		}
		return []string{"<event>"}
	case "warning":
		lines := []string{"<warning>"}
		if e.Body != "" {
			lines = append(lines, e.Body)
		}
		return lines
	case "error":
		lines := []string{"<error>"}
		if e.Body != "" {
			lines = append(lines, e.Body)
		}
		return lines
	default:
		if e.Body != "" {
			return []string{e.Body}
		}
		if e.Heading != "" {
			return []string{e.Heading}
		}
		return nil
	}
}

// renderToolUseTag converts a tool_use heading+body into an XML-style tag.
// The heading field has the "### " prefix already stripped by splitFormatted.
func renderToolUseTag(heading, body string) string {
	switch {
	case strings.HasPrefix(heading, "Bash"):
		desc := ""
		if i := strings.Index(heading, " — "); i >= 0 {
			desc = heading[i+len(" — "):]
		}
		if desc != "" {
			return "<bash description=\"" + desc + "\">"
		}
		return "<bash>"
	case heading == "Read":
		if body != "" {
			return "<read path=\"" + strings.TrimSpace(body) + "\">"
		}
		return "<read>"
	case heading == "Write":
		if body != "" {
			return "<write path=\"" + strings.TrimSpace(body) + "\">"
		}
		return "<write>"
	case heading == "Edit":
		if body != "" {
			return "<edit path=\"" + strings.TrimSpace(body) + "\">"
		}
		return "<edit>"
	case heading == "Glob":
		if body != "" {
			return "<glob pattern=\"" + strings.TrimSpace(body) + "\">"
		}
		return "<glob>"
	case heading == "Grep":
		if body != "" {
			parts := strings.SplitN(strings.TrimSpace(body), " in ", 2)
			if len(parts) == 2 {
				return "<grep pattern=\"" + parts[0] + "\" path=\"" + parts[1] + "\">"
			}
			return "<grep pattern=\"" + parts[0] + "\">"
		}
		return "<grep>"
	default:
		name := strings.ToLower(heading)
		return "<" + name + ">"
	}
}

// toolBodyInTag returns true if the tool's body content is consumed into the tag attributes
// (and should not be repeated as a separate line).
func toolBodyInTag(heading string) bool {
	switch heading {
	case "Read", "Write", "Edit", "Glob", "Grep":
		return true
	}
	return false
}

// RenderEntries renders all entries into a single string, suitable for file dumps.
func RenderEntries(entries []db.LogEntry) string {
	var all []string
	for _, e := range entries {
		lines := RenderEntry(e)
		all = append(all, lines...)
		all = append(all, "") // entry separator
	}
	return strings.Join(all, "\n") + "\n"
}
