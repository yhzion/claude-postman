// Package service handles system service (systemd/launchd) integration.
package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
)

const (
	systemdPath = "/etc/systemd/system/claude-postman.service"
	plistName   = "com.claude-postman.plist"
)

func generateSystemdUnit(binaryPath, userName, home string) string {
	return fmt.Sprintf(`[Unit]
Description=claude-postman
After=network.target

[Service]
Type=simple
User=%s
ExecStart=%s serve
Restart=on-failure
RestartSec=5
Environment=HOME=%s

[Install]
WantedBy=multi-user.target
`, userName, binaryPath, home)
}

func generateLaunchdPlist(binaryPath, dataDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.claude-postman</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/claude-postman.log</string>
    <key>StandardErrorPath</key>
    <string>%s/claude-postman.err</string>
</dict>
</plist>
`, binaryPath, dataDir, dataDir)
}

func selfPath() (string, error) {
	return os.Executable()
}

// resolveUser returns the real (non-root) user name and home directory.
// When running under sudo, SUDO_USER holds the original user.
func resolveUser() (string, string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err := user.Lookup(sudoUser)
		if err != nil {
			return "", "", fmt.Errorf("lookup SUDO_USER %s: %w", sudoUser, err)
		}
		return u.Username, u.HomeDir, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", "", fmt.Errorf("get current user: %w", err)
	}
	return u.Username, u.HomeDir, nil
}

// InstallService registers claude-postman as a system service.
func InstallService() error {
	bin, err := selfPath()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	switch runtime.GOOS {
	case "linux":
		return installSystemd(bin)
	case "darwin":
		return installLaunchd(bin)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// UninstallService removes the claude-postman system service.
func UninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallSystemd()
	case "darwin":
		return uninstallLaunchd()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// RestartAfterUpdate restarts the service if installed.
// On Linux (systemd), prints a sudo command since root is required.
// On macOS (launchd), restarts directly.
// Does nothing if no service is installed.
func RestartAfterUpdate() {
	switch runtime.GOOS {
	case "linux":
		restartSystemd()
	case "darwin":
		restartLaunchd()
	}
}

func restartSystemd() {
	if _, err := os.Stat(systemdPath); err != nil {
		fmt.Println("\nIf claude-postman serve is running, restart it to apply the update.")
		return
	}
	fmt.Println("\nService is installed. Restart to apply the update:")
	fmt.Println("  sudo systemctl restart claude-postman")
}

func restartLaunchd() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", plistName)
	if _, err := os.Stat(plistPath); err != nil {
		fmt.Println("\nIf claude-postman serve is running, restart it to apply the update.")
		return
	}
	fmt.Print("\nRestarting service... ")
	_ = exec.Command("launchctl", "stop", "com.claude-postman").Run()
	if err := exec.Command("launchctl", "start", "com.claude-postman").Run(); err != nil {
		fmt.Printf("✗ %v\n", err)
		fmt.Println("  Run manually: launchctl stop com.claude-postman && launchctl start com.claude-postman")
		return
	}
	fmt.Println("✅ restarted")
}

func installSystemd(bin string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required.\n  Run: sudo %s install-service", bin)
	}

	userName, home, err := resolveUser()
	if err != nil {
		return err
	}

	content := generateSystemdUnit(bin, userName, home)
	if err := os.WriteFile(systemdPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "claude-postman"},
		{"systemctl", "start", "claude-postman"},
	}
	for _, args := range cmds {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil { //nolint:gosec // args are internally controlled
			return fmt.Errorf("%s: %w", args[0], err)
		}
	}

	fmt.Println("Service installed and started.")
	return nil
}

func uninstallSystemd() error {
	if os.Geteuid() != 0 {
		bin, _ := os.Executable()
		return fmt.Errorf("root privileges required.\n  Run: sudo %s uninstall-service", bin)
	}

	cmds := [][]string{
		{"systemctl", "stop", "claude-postman"},
		{"systemctl", "disable", "claude-postman"},
	}
	for _, args := range cmds {
		_ = exec.Command(args[0], args[1:]...).Run() //nolint:gosec // args are internally controlled
	}
	if err := os.Remove(systemdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service file: %w", err)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println("Service uninstalled.")
	return nil
}

func installLaunchd(bin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	dataDir := filepath.Join(home, ".claude-postman", "data")
	plistPath := filepath.Join(home, "Library", "LaunchAgents", plistName)

	content := generateLaunchdPlist(bin, dataDir)
	if err := os.WriteFile(plistPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}

	fmt.Println("Service installed and loaded.")
	return nil
}

func uninstallLaunchd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", plistName)

	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}

	fmt.Println("Service uninstalled.")
	return nil
}
