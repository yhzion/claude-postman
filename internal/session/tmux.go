package session

import (
	"fmt"
	"os/exec"
)

// TmuxRunner abstracts tmux command execution for testability.
type TmuxRunner interface {
	NewSession(name, workingDir string) error
	SendKeys(sessionName, text string) error
	CapturePane(sessionName string, lines int) (string, error)
	KillSession(sessionName string) error
	HasSession(sessionName string) bool
}

type tmuxCmd struct{}

// NewTmuxRunner returns a TmuxRunner that executes real tmux commands.
func NewTmuxRunner() TmuxRunner {
	return &tmuxCmd{}
}

func (t *tmuxCmd) NewSession(name, workingDir string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workingDir).Run()
}

func (t *tmuxCmd) SendKeys(sessionName, text string) error {
	return exec.Command("tmux", "send-keys", "-t", sessionName, text, "Enter").Run()
}

func (t *tmuxCmd) CapturePane(sessionName string, lines int) (string, error) {
	arg := fmt.Sprintf("-%d", lines)
	out, err := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", arg).Output() //nolint:gosec // args are internally controlled
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *tmuxCmd) KillSession(sessionName string) error {
	return exec.Command("tmux", "kill-session", "-t", sessionName).Run()
}

func (t *tmuxCmd) HasSession(sessionName string) bool {
	return exec.Command("tmux", "has-session", "-t", sessionName).Run() == nil
}
