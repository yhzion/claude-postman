// Package doctor provides environment diagnostics for claude-postman.
package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	statusOK    = "ok"
	statusError = "error"
	statusWarn  = "warn"
	statusFixed = "fixed"
)

// CheckResult holds the outcome of a single diagnostic check.
type CheckResult struct {
	Name    string
	Status  string
	Message string
	Hint    string
}

func checkConfig(configDir string) CheckResult {
	path := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(path); err != nil {
		return CheckResult{
			Name:    "Config",
			Status:  statusError,
			Message: path + ": not found",
			Hint:    "Run 'claude-postman init' to create one",
		}
	}
	return CheckResult{Name: "Config", Status: statusOK, Message: path}
}

func checkDataDir(dataDir string, fix bool) CheckResult {
	if info, err := os.Stat(dataDir); err == nil && info.IsDir() {
		return CheckResult{Name: "Data directory", Status: statusOK, Message: dataDir}
	}
	if fix {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return CheckResult{
				Name:    "Data directory",
				Status:  statusError,
				Message: "not found, fix failed: " + err.Error(),
			}
		}
		return CheckResult{
			Name:    "Data directory",
			Status:  statusFixed,
			Message: "not found → Created",
		}
	}
	return CheckResult{
		Name:    "Data directory",
		Status:  statusError,
		Message: dataDir + ": not found",
		Hint:    "Run 'claude-postman doctor --fix' to create",
	}
}

func checkCommand(name, bin, versionFlag string) CheckResult {
	out, err := exec.Command(bin, versionFlag).CombinedOutput() //nolint:gosec // args are internally controlled
	if err != nil {
		return CheckResult{
			Name:    name,
			Status:  statusError,
			Message: "not found",
			Hint:    fmt.Sprintf("Install %s", name),
		}
	}
	ver := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return CheckResult{Name: name, Status: statusOK, Message: ver}
}

func icon(status string) string {
	switch status {
	case statusOK, statusFixed:
		return "✅"
	case statusWarn:
		return "⚠️ "
	default:
		return "❌"
	}
}

func printResult(w io.Writer, r CheckResult) {
	fmt.Fprintf(w, "  %s %s: %s\n", icon(r.Status), r.Name, r.Message)
}

type summary struct {
	errors, warnings, fixed int
}

func countResults(results []CheckResult) summary {
	var s summary
	for _, r := range results {
		switch r.Status {
		case statusError:
			s.errors++
		case statusWarn:
			s.warnings++
		case statusFixed:
			s.fixed++
		}
	}
	return s
}

func printHints(w io.Writer, results []CheckResult) {
	var any bool
	for _, r := range results {
		if r.Hint != "" && (r.Status == statusError || r.Status == statusWarn) {
			if !any {
				fmt.Fprintln(w)
				any = true
			}
			fmt.Fprintf(w, "  %s %s: %s\n", icon(r.Status), r.Name, r.Hint)
		}
	}
}

// RunDoctor runs all diagnostic checks and writes results to w.
// Returns exit code: 0=all pass, 1=errors, 2=warnings only.
func RunDoctor(w io.Writer, configDir, dataDir string, fix bool) int {
	fmt.Fprintln(w, "Checking environment...")
	fmt.Fprintln(w)

	results := []CheckResult{
		checkConfig(configDir),
		checkDataDir(dataDir, fix),
		checkCommand("tmux", "tmux", "-V"),
		checkCommand("Claude Code", "claude", "--version"),
	}
	for _, r := range results {
		printResult(w, r)
	}

	s := countResults(results)
	printHints(w, results)

	if s.fixed > 0 {
		fmt.Fprintf(w, "\nFixed %d issue(s).", s.fixed)
		if s.errors > 0 || s.warnings > 0 {
			fmt.Fprintf(w, " %d error(s), %d warning(s) remaining.", s.errors, s.warnings)
		}
		fmt.Fprintln(w)
	}

	if s.errors > 0 {
		return 1
	}
	if s.warnings > 0 {
		return 2
	}
	return 0
}
