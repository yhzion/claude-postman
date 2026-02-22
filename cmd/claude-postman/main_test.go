package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCmd_HasExpectedSubcommands(t *testing.T) {
	root := newRootCmd()
	names := make(map[string]bool)
	for _, cmd := range root.Commands() {
		names[cmd.Name()] = true
	}

	expected := []string{"init", "serve", "doctor", "install-service", "uninstall-service", "update", "uninstall"}
	for _, name := range expected {
		assert.True(t, names[name], "missing subcommand: %s", name)
	}
}

func TestRootCmd_HasVersion(t *testing.T) {
	root := newRootCmd()
	assert.NotEmpty(t, root.Version)
}

func TestUninstallCmd_HasYesFlag(t *testing.T) {
	root := newRootCmd()
	var uninstallCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "uninstall" {
			uninstallCmd = cmd
			break
		}
	}
	require.NotNil(t, uninstallCmd)
	f := uninstallCmd.Flags().Lookup("yes")
	assert.NotNil(t, f, "--yes flag should exist")
}

func TestDoctorCmd_HasFixFlag(t *testing.T) {
	root := newRootCmd()
	var doctorCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "doctor" {
			doctorCmd = cmd
			break
		}
	}
	require.NotNil(t, doctorCmd)
	f := doctorCmd.Flags().Lookup("fix")
	assert.NotNil(t, f, "--fix flag should exist")
}
