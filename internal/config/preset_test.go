package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreset_Gmail(t *testing.T) {
	// Gmail 프리셋이 존재하고 올바른 값을 가지는지 확인
	preset, ok := Presets["gmail"]
	require.True(t, ok, "Gmail 프리셋이 존재해야 함")

	assert.Equal(t, "smtp.gmail.com", preset.SMTPHost)
	assert.Equal(t, 587, preset.SMTPPort)
	assert.Equal(t, "imap.gmail.com", preset.IMAPHost)
	assert.Equal(t, 993, preset.IMAPPort)
}

func TestPreset_Outlook(t *testing.T) {
	// Outlook 프리셋이 존재하고 올바른 값을 가지는지 확인
	preset, ok := Presets["outlook"]
	require.True(t, ok, "Outlook 프리셋이 존재해야 함")

	assert.Equal(t, "smtp.office365.com", preset.SMTPHost)
	assert.Equal(t, 587, preset.SMTPPort)
	assert.Equal(t, "outlook.office365.com", preset.IMAPHost)
	assert.Equal(t, 993, preset.IMAPPort)
}
