package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSystemdUnit(t *testing.T) {
	unit := generateSystemdUnit("/usr/local/bin/claude-postman", "testuser", "/home/testuser")

	assert.Contains(t, unit, "Description=claude-postman")
	assert.Contains(t, unit, "ExecStart=/usr/local/bin/claude-postman serve")
	assert.Contains(t, unit, "User=testuser")
	assert.Contains(t, unit, "Environment=HOME=/home/testuser")
	assert.Contains(t, unit, "After=network.target")
	assert.Contains(t, unit, "Restart=on-failure")
	assert.Contains(t, unit, "WantedBy=multi-user.target")
}

func TestGenerateLaunchdPlist(t *testing.T) {
	plist := generateLaunchdPlist("/usr/local/bin/claude-postman", "/home/testuser/.claude-postman/data")

	assert.Contains(t, plist, "<string>com.claude-postman</string>")
	assert.Contains(t, plist, "<string>/usr/local/bin/claude-postman</string>")
	assert.Contains(t, plist, "<string>serve</string>")
	assert.Contains(t, plist, "<true/>") // RunAtLoad or KeepAlive
	assert.Contains(t, plist, "claude-postman.log")
	assert.Contains(t, plist, "claude-postman.err")
}

func TestGenerateSystemdUnit_Structure(t *testing.T) {
	unit := generateSystemdUnit("/bin/cp", "u", "/h")
	sections := []string{"[Unit]", "[Service]", "[Install]"}
	for _, s := range sections {
		assert.True(t, strings.Contains(unit, s), "missing section: %s", s)
	}
}

func TestGenerateLaunchdPlist_XMLValid(t *testing.T) {
	plist := generateLaunchdPlist("/bin/cp", "/tmp/data")
	assert.True(t, strings.HasPrefix(plist, "<?xml"))
	assert.Contains(t, plist, "</plist>")
}
