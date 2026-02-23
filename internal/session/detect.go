package session

import "strings"

// HasInputPrompt checks the last 5 lines of tmux output for Claude Code's
// input prompt indicator (❯). Returns true if Claude Code is waiting for input.
func HasInputPrompt(output string) bool {
	output = strings.TrimRight(output, " \t\n")
	if output == "" {
		return false
	}
	lines := strings.Split(output, "\n")
	start := len(lines) - 5
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		if strings.Contains(lines[i], "❯") {
			return true
		}
	}
	return false
}
