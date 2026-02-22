package email

import (
	"html"
	"regexp"
	"strings"
)

var (
	sessionIDRe = regexp.MustCompile(`Session-ID:\s*([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)
	dirRe       = regexp.MustCompile(`(?m)^Directory:\s*(.+)$`)
	modelRe     = regexp.MustCompile(`(?m)^Model:\s*(.+)$`)
	tagRe       = regexp.MustCompile(`<[^>]*>`)
	blockRe     = regexp.MustCompile(`(?i)<\s*(?:br|/p|/div|/tr|/li)\s*/?\s*>`)
)

const forwardedMarker = "---------- Forwarded message ----------"

// ParseSessionID extracts a Session-ID UUID from the email body.
// Returns empty string if not found.
func ParseSessionID(body string) string {
	m := sessionIDRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

// ParseTemplate applies the template parsing pipeline:
//  1. Remove forwarded message section
//  2. Remove quote prefixes (max 1 level)
//  3. Extract Directory, Model, and remaining text as prompt
//
// Returns empty strings for workingDir/model if not found (caller applies defaults).
func ParseTemplate(body string) (workingDir, model, prompt string) {
	// Step 2: Remove forwarded message section
	if idx := strings.Index(body, forwardedMarker); idx >= 0 {
		body = body[:idx]
	}

	// Step 3: Remove quote prefixes (max 1 level)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "> ") {
			lines[i] = line[2:]
		} else if strings.HasPrefix(line, ">") {
			lines[i] = line[1:]
		}
	}
	body = strings.Join(lines, "\n")

	// Step 4: Trim whitespace
	body = strings.TrimSpace(body)

	// Extract Directory
	if m := dirRe.FindStringSubmatch(body); m != nil {
		workingDir = strings.TrimSpace(m[1])
		body = dirRe.ReplaceAllString(body, "")
	}

	// Extract Model
	if m := modelRe.FindStringSubmatch(body); m != nil {
		model = strings.TrimSpace(m[1])
		body = modelRe.ReplaceAllString(body, "")
	}

	// Remaining text is the prompt
	prompt = strings.TrimSpace(body)
	return
}

// ExtractTextFromHTML strips HTML tags and returns plain text.
func ExtractTextFromHTML(s string) string {
	// Replace block-level closing tags and <br> with newlines
	text := blockRe.ReplaceAllString(s, "\n")
	// Remove all remaining tags
	text = tagRe.ReplaceAllString(text, "")
	// Decode HTML entities
	text = html.UnescapeString(text)
	// Normalize whitespace within lines
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")
	// Collapse multiple blank lines
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}
