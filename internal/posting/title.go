package posting

import (
	"fmt"
	"strings"
	"time"
)

// TitleVars holds the named variables available for title template rendering.
type TitleVars struct {
	Date    time.Time // UTC time used for {{date}} and {{date:FORMAT}}
	Repo    string    // "owner/repo"
	Owner   string    // "owner"
	Command string    // "report", "lead-time", etc.
}

// Supported variable names (without format qualifiers).
var knownVars = map[string]bool{
	"date":    true,
	"repo":    true,
	"owner":   true,
	"command": true,
}

// RenderTitle replaces {{var}} placeholders in template with values from vars.
//
// Supported variables:
//
//	{{date}}            — UTC date in 2006-01-02 format
//	{{date:FORMAT}}     — UTC date in custom Go time.Format layout
//	{{repo}}            — owner/repo
//	{{owner}}           — repository owner
//	{{command}}         — command name (report, lead-time, etc.)
func RenderTitle(template string, vars TitleVars) string {
	var b strings.Builder
	rest := template
	for {
		open := strings.Index(rest, "{{")
		if open == -1 {
			b.WriteString(rest)
			break
		}
		close := strings.Index(rest[open:], "}}")
		if close == -1 {
			b.WriteString(rest)
			break
		}
		close += open

		b.WriteString(rest[:open])
		placeholder := strings.TrimSpace(rest[open+2 : close])
		b.WriteString(resolvePlaceholder(placeholder, vars))
		rest = rest[close+2:]
	}
	return b.String()
}

func resolvePlaceholder(placeholder string, vars TitleVars) string {
	// Check for format qualifier: "date:Jan 2, 2006"
	if name, format, ok := strings.Cut(placeholder, ":"); ok {
		name = strings.TrimSpace(name)
		format = strings.TrimSpace(format)
		if name == "date" && format != "" {
			return vars.Date.Format(format)
		}
		// Unknown qualified variable — return as-is for visibility.
		return "{{" + placeholder + "}}"
	}

	switch placeholder {
	case "date":
		return vars.Date.Format("2006-01-02")
	case "repo":
		return vars.Repo
	case "owner":
		return vars.Owner
	case "command":
		return vars.Command
	default:
		// Unknown variable — return as-is for visibility.
		return "{{" + placeholder + "}}"
	}
}

// ValidateTitleTemplate checks that template has balanced {{ }} delimiters,
// uses only known variable names, and renders to a non-empty string.
func ValidateTitleTemplate(template string) error {
	if template == "" {
		return fmt.Errorf("discussions.title must be non-empty")
	}

	rest := template
	for {
		open := strings.Index(rest, "{{")
		if open == -1 {
			break
		}
		close := strings.Index(rest[open:], "}}")
		if close == -1 {
			return fmt.Errorf("discussions.title has unclosed {{ delimiter")
		}
		close += open
		inner := rest[open+2 : close]
		if strings.Contains(inner, "{{") {
			return fmt.Errorf("discussions.title has nested {{ delimiters")
		}

		// Validate the variable name.
		varName := strings.TrimSpace(inner)
		if name, _, ok := strings.Cut(varName, ":"); ok {
			varName = strings.TrimSpace(name)
		}
		if !knownVars[varName] {
			return fmt.Errorf("discussions.title has unknown variable %q (supported: date, repo, owner, command)", varName)
		}

		rest = rest[close+2:]
	}

	if strings.Contains(rest, "}}") {
		return fmt.Errorf("discussions.title has unmatched }} delimiter")
	}

	// Test render with dummy values to catch empty results.
	rendered := RenderTitle(template, TitleVars{
		Date:    time.Now().UTC(),
		Repo:    "test/repo",
		Owner:   "test",
		Command: "report",
	})
	if strings.TrimSpace(rendered) == "" {
		return fmt.Errorf("discussions.title renders to empty string")
	}
	if len(rendered) > 256 {
		return fmt.Errorf("discussions.title renders to %d characters, max 256", len(rendered))
	}

	return nil
}
