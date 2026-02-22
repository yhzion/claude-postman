package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
