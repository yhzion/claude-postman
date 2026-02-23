package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripANSI(t *testing.T) {
	t.Run("removes color codes", func(t *testing.T) {
		input := "\x1b[32mgreen\x1b[0m normal"
		assert.Equal(t, "green normal", StripANSI(input))
	})

	t.Run("removes bold and cursor codes", func(t *testing.T) {
		input := "\x1b[1mbold\x1b[0m \x1b[2K\x1b[1Gcursor"
		assert.Equal(t, "bold cursor", StripANSI(input))
	})

	t.Run("preserves text without ANSI", func(t *testing.T) {
		input := "Hello world\nSecond line"
		assert.Equal(t, input, StripANSI(input))
	})

	t.Run("handles empty string", func(t *testing.T) {
		assert.Equal(t, "", StripANSI(""))
	})

	t.Run("removes multi-param codes", func(t *testing.T) {
		input := "\x1b[38;5;196mred text\x1b[0m"
		assert.Equal(t, "red text", StripANSI(input))
	})
}

func TestRenderHTML(t *testing.T) {
	t.Run("basic markdown to HTML", func(t *testing.T) {
		html, err := RenderHTML("Hello **world**")
		require.NoError(t, err)
		assert.Contains(t, html, "<strong>world</strong>")
	})

	t.Run("code blocks with syntax highlighting", func(t *testing.T) {
		md := "```go\nfmt.Println(\"hello\")\n```"
		html, err := RenderHTML(md)
		require.NoError(t, err)
		assert.Contains(t, html, "Println")
	})

	t.Run("wraps in HTML email template", func(t *testing.T) {
		html, err := RenderHTML("test")
		require.NoError(t, err)
		assert.Contains(t, html, "<html>")
		assert.Contains(t, html, "</html>")
		assert.Contains(t, html, "<body")
	})
}
