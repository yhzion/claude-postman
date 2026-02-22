// Package doctor provides environment diagnostics for claude-postman.
package doctor

import (
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for DB check
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

// Dialer abstracts TCP dial for testability.
type Dialer func(network, addr string, timeout time.Duration) (net.Conn, error)

// Deps holds injectable dependencies for doctor checks.
type Deps struct {
	ConfigDir string
	DataDir   string
	SMTPAddr  string // "host:port", empty to skip
	IMAPAddr  string // "host:port", empty to skip
	Dial      Dialer // nil uses net.DialTimeout
}

func (d *Deps) dial() Dialer {
	if d.Dial != nil {
		return d.Dial
	}
	return net.DialTimeout
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

func checkSQLite(dataDir string, fix bool) CheckResult {
	dbPath := filepath.Join(dataDir, "claude-postman.db")

	if _, err := os.Stat(dbPath); err != nil {
		if !fix {
			return CheckResult{
				Name:    "Database",
				Status:  statusError,
				Message: "not found",
				Hint:    "Run 'claude-postman doctor --fix' to initialize",
			}
		}
		// --fix: create and migrate
		return initDB(dbPath)
	}

	// DB exists, check migration status
	return checkMigration(dbPath, fix)
}

func initDB(dbPath string) CheckResult {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return CheckResult{Name: "Database", Status: statusError, Message: "open failed: " + err.Error()}
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return CheckResult{Name: "Database", Status: statusError, Message: "ping failed: " + err.Error()}
	}
	return CheckResult{Name: "Database", Status: statusFixed, Message: "not found → Initialized"}
}

func checkMigration(dbPath string, fix bool) CheckResult {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return CheckResult{Name: "Database", Status: statusError, Message: "open failed: " + err.Error()}
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return CheckResult{Name: "Database", Status: statusError, Message: "ping failed: " + err.Error()}
	}

	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&name)
	if err == sql.ErrNoRows {
		if fix {
			return CheckResult{Name: "Database", Status: statusFixed, Message: "not migrated → Initialized"}
		}
		return CheckResult{
			Name:    "Database",
			Status:  statusError,
			Message: "not migrated",
			Hint:    "Run 'claude-postman doctor --fix' to migrate",
		}
	}
	if err != nil {
		return CheckResult{Name: "Database", Status: statusError, Message: "query failed: " + err.Error()}
	}

	var ver int
	err = db.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&ver)
	if err != nil {
		return CheckResult{Name: "Database", Status: statusOK, Message: "OK"}
	}
	return CheckResult{Name: "Database", Status: statusOK, Message: fmt.Sprintf("OK (version %d)", ver)}
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

func checkTCPService(name, addr string, dial Dialer) CheckResult {
	if addr == "" {
		return CheckResult{Name: name, Status: statusError, Message: "not configured", Hint: "Check " + name + " settings"}
	}
	conn, err := dial("tcp", addr, 5*time.Second)
	if err != nil {
		return CheckResult{Name: name, Status: statusError, Message: addr + " (connection failed)", Hint: "Check " + name + " settings"}
	}
	conn.Close()
	return CheckResult{Name: name, Status: statusOK, Message: addr + " (connected)"}
}

func checkService() CheckResult {
	switch runtime.GOOS {
	case "linux":
		return checkSystemdService()
	case "darwin":
		return checkLaunchdService()
	default:
		return CheckResult{Name: "Service", Status: statusWarn, Message: "unsupported platform"}
	}
}

func checkSystemdService() CheckResult {
	const path = "/etc/systemd/system/claude-postman.service"
	if _, err := os.Stat(path); err != nil {
		return CheckResult{
			Name:    "Service",
			Status:  statusWarn,
			Message: "not registered",
			Hint:    "Run 'sudo claude-postman install-service' to register",
		}
	}
	return CheckResult{Name: "Service", Status: statusOK, Message: "registered"}
}

func checkLaunchdService() CheckResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return CheckResult{Name: "Service", Status: statusWarn, Message: "cannot detect"}
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "com.claude-postman.plist")
	if _, err := os.Stat(path); err != nil {
		return CheckResult{
			Name:    "Service",
			Status:  statusWarn,
			Message: "not registered",
			Hint:    "Run 'claude-postman install-service' to register",
		}
	}
	return CheckResult{Name: "Service", Status: statusOK, Message: "registered"}
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

type resultSummary struct {
	errors, warnings, fixed int
}

func countResults(results []CheckResult) resultSummary {
	var s resultSummary
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

func printSummary(w io.Writer, s resultSummary) {
	if s.fixed > 0 {
		fmt.Fprintf(w, "\nFixed %d issue(s).", s.fixed)
		if s.errors > 0 || s.warnings > 0 {
			fmt.Fprintf(w, " %d error(s), %d warning(s) remaining.", s.errors, s.warnings)
		}
		fmt.Fprintln(w)
	}
}

// RunDoctor runs all diagnostic checks and writes results to w.
// Returns exit code: 0=all pass, 1=errors, 2=warnings only.
func RunDoctor(w io.Writer, deps Deps, fix bool) int {
	fmt.Fprintln(w, "Checking environment...")
	fmt.Fprintln(w)

	results := []CheckResult{
		checkConfig(deps.ConfigDir),
		checkDataDir(deps.DataDir, fix),
		checkSQLite(deps.DataDir, fix),
		checkCommand("tmux", "tmux", "-V"),
		checkCommand("Claude Code", "claude", "--version"),
		checkTCPService("SMTP", deps.SMTPAddr, deps.dial()),
		checkTCPService("IMAP", deps.IMAPAddr, deps.dial()),
		checkService(),
	}
	for _, r := range results {
		printResult(w, r)
	}

	s := countResults(results)
	printHints(w, results)
	printSummary(w, s)

	if s.errors > 0 {
		return 1
	}
	if s.warnings > 0 {
		return 2
	}
	return 0
}
