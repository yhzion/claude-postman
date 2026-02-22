// Package main is the entry point for claude-postman.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/yhzion/claude-postman/internal/config"
	"github.com/yhzion/claude-postman/internal/doctor"
	"github.com/yhzion/claude-postman/internal/email"
	"github.com/yhzion/claude-postman/internal/serve"
	"github.com/yhzion/claude-postman/internal/service"
	"github.com/yhzion/claude-postman/internal/session"
	"github.com/yhzion/claude-postman/internal/storage"
	"github.com/yhzion/claude-postman/internal/updater"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "claude-postman",
		Short: "claude-postman - Email relay for Claude Code",
		Long:  "claude-postman - Email relay for Claude Code",
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			// Check for updates after any command (non-blocking)
			go updater.New(version).CheckAndNotify()
		},
	}
	root.Version = version
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newInitCmd(),
		newServeCmd(),
		newDoctorCmd(),
		newSendTemplateCmd(),
		newInstallServiceCmd(),
		newUninstallServiceCmd(),
		newUpdateCmd(),
		newUninstallCmd(),
	)
	return root
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Setup configuration wizard",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.RunInit()
			if err != nil {
				return err
			}
			sendInitTemplate(cfg)
			fmt.Println("\nRun 'claude-postman serve' to start.")
			return nil
		},
	}
}

// sendInitTemplate sends the session creation template email after init.
// Errors are logged but do not fail the init process.
func sendInitTemplate(cfg *config.Config) {
	store, err := storage.New(cfg.General.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n⚠ Could not open database: %v\n", err)
		return
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "\n⚠ Could not migrate database: %v\n", err)
		return
	}

	mailer := email.New(&cfg.Email, store)
	fmt.Print("\nSending session template email... ")
	if _, err := mailer.SendTemplate(); err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		fmt.Fprintln(os.Stderr, "  You can re-run 'claude-postman init' to retry.")
		return
	}
	fmt.Println("✅ sent")
	fmt.Println("  Forward this email to create new Claude Code sessions.")
}

func newSendTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send-template",
		Short: "Send a session creation template email",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			store, err := storage.New(cfg.General.DataDir)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer store.Close()

			if err := store.Migrate(); err != nil {
				return fmt.Errorf("migrate database: %w", err)
			}

			mailer := email.New(&cfg.Email, store)
			fmt.Print("Sending session template email... ")
			msgID, err := mailer.SendTemplate()
			if err != nil {
				return fmt.Errorf("send template: %w", err)
			}
			fmt.Println("✅ sent")
			fmt.Printf("  Message-ID: %s\n", msgID)
			fmt.Println("  Forward this email to create new Claude Code sessions.")
			return nil
		},
	}
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the relay server",
		RunE:  runServe,
	}
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check environment and diagnose issues",
	}
	fix := cmd.Flags().Bool("fix", false, "Attempt to automatically fix issues")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		return runDoctor(*fix)
	}
	return cmd
}

func newInstallServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install-service",
		Short: "Register as a system service (Linux requires sudo)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return service.InstallService()
		},
	}
}

func newUninstallServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall-service",
		Short: "Remove system service (Linux requires sudo)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return service.UninstallService()
		},
	}
}

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update to the latest version",
		RunE: func(_ *cobra.Command, _ []string) error {
			return updater.New(version).RunUpdate()
		},
	}
}

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove claude-postman from this system",
	}
	yes := cmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		return runUninstall(*yes)
	}
	return cmd
}

func runUninstall(yes bool) error {
	if !yes {
		fmt.Print("This will remove claude-postman, its config, and all data. Continue? [y/N] ")
		var answer string
		fmt.Scanln(&answer) //nolint:errcheck // user input prompt, error is not actionable
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// 1. Stop and remove system service
	fmt.Println("Removing system service...")
	_ = service.UninstallService()

	// 2. Remove config/data directory
	configDir := config.ConfigDir()
	if configDir != "" {
		fmt.Printf("Removing %s...\n", configDir)
		if err := os.RemoveAll(configDir); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		}
	}

	// 3. Remove binary
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	fmt.Printf("Removing %s...\n", bin)
	if err := os.Remove(bin); err != nil {
		return fmt.Errorf("remove binary: %w (try: sudo claude-postman uninstall --yes)", err)
	}

	fmt.Println("claude-postman has been uninstalled.")
	return nil
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := storage.New(cfg.General.DataDir)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	tmux := session.NewTmuxRunner()
	mgr := session.New(store, tmux)
	mailer := email.New(&cfg.Email, store)

	return serve.RunServe(context.Background(), cfg, store, mgr, mailer)
}

func runDoctor(fix bool) error {
	configDir := config.ConfigDir()
	deps := doctor.Deps{ConfigDir: configDir}

	cfg, err := config.Load()
	if err == nil {
		deps.DataDir = cfg.General.DataDir
		deps.SMTPAddr = fmt.Sprintf("%s:%d", cfg.Email.SMTPHost, cfg.Email.SMTPPort)
		deps.IMAPAddr = fmt.Sprintf("%s:%d", cfg.Email.IMAPHost, cfg.Email.IMAPPort)
	} else {
		deps.DataDir = configDir + "/data"
	}

	exitCode := doctor.RunDoctor(os.Stdout, deps, fix)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
