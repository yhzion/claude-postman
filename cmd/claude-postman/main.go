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
)

var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "claude-postman",
		Short: "claude-postman - Email relay for Claude Code",
		Long:  "claude-postman - Email relay for Claude Code",
	}
	root.Version = version
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newInitCmd(),
		newServeCmd(),
		newDoctorCmd(),
		newInstallServiceCmd(),
		newUninstallServiceCmd(),
	)
	return root
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Setup configuration wizard",
		RunE: func(_ *cobra.Command, _ []string) error {
			return config.RunInit()
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
		Short: "Register as a system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return service.InstallService()
		},
	}
}

func newUninstallServiceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall-service",
		Short: "Remove system service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return service.UninstallService()
		},
	}
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
	dataDir := ""

	cfg, err := config.Load()
	if err == nil {
		dataDir = cfg.General.DataDir
	} else {
		// Fallback: use default data dir
		dataDir = configDir + "/data"
	}

	exitCode := doctor.RunDoctor(os.Stdout, configDir, dataDir, fix)
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
