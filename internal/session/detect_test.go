package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasInputPrompt(t *testing.T) {
	t.Run("detects prompt on last line", func(t *testing.T) {
		output := "Some output\n❯ "
		assert.True(t, HasInputPrompt(output))
	})

	t.Run("detects prompt within last 5 lines", func(t *testing.T) {
		output := "line1\nline2\nline3\n❯ \n\n"
		assert.True(t, HasInputPrompt(output))
	})

	t.Run("no prompt when thinking", func(t *testing.T) {
		output := "● Thinking about the problem...\n  Still working..."
		assert.False(t, HasInputPrompt(output))
	})

	t.Run("no prompt in middle of output", func(t *testing.T) {
		output := "❯ old prompt\nNow doing work\nMore output\nStill going\nAlmost done\nFinishing up"
		assert.False(t, HasInputPrompt(output))
	})

	t.Run("empty output returns false", func(t *testing.T) {
		assert.False(t, HasInputPrompt(""))
	})

	t.Run("only whitespace returns false", func(t *testing.T) {
		assert.False(t, HasInputPrompt("   \n  \n  "))
	})

	t.Run("detects prompt with text after it", func(t *testing.T) {
		output := "Choose a project:\n1. A\n2. B\n❯ waiting for input"
		assert.True(t, HasInputPrompt(output))
	})
}
