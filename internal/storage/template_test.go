package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveTemplate_IsValidTemplateRef(t *testing.T) {
	store := newTestStore(t)

	tmpl := &Template{
		ID:        "tmpl-1",
		MessageID: "msg-id-12345",
	}
	err := store.SaveTemplate(tmpl)
	require.NoError(t, err)

	valid, err := store.IsValidTemplateRef("msg-id-12345")
	require.NoError(t, err)
	assert.True(t, valid, "저장된 messageID는 유효해야 함")
}

func TestIsValidTemplateRef_NotFound(t *testing.T) {
	store := newTestStore(t)

	valid, err := store.IsValidTemplateRef("nonexistent-msg-id")
	require.NoError(t, err)
	assert.False(t, valid, "존재하지 않는 messageID는 false 반환")
}
