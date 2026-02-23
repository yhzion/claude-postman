package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSessionID(t *testing.T) {
	t.Run("extracts Session-ID from body", func(t *testing.T) {
		body := "Some text\nSession-ID: 123e4567-e89b-12d3-a456-426614174000\nMore text"
		assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", ParseSessionID(body))
	})

	t.Run("returns empty when no Session-ID", func(t *testing.T) {
		assert.Equal(t, "", ParseSessionID("no session id here"))
	})

	t.Run("handles Session-ID at start of body", func(t *testing.T) {
		body := "Session-ID: abcdef01-2345-6789-abcd-ef0123456789"
		assert.Equal(t, "abcdef01-2345-6789-abcd-ef0123456789", ParseSessionID(body))
	})
}

func TestParseTemplate(t *testing.T) {
	t.Run("extracts Directory, Model, and prompt", func(t *testing.T) {
		body := "Directory: /home/user\nModel: opus\n\nDo something cool"
		dir, model, prompt := ParseTemplate(body)
		assert.Equal(t, "/home/user", dir)
		assert.Equal(t, "opus", model)
		assert.Equal(t, "Do something cool", prompt)
	})

	t.Run("missing Directory returns empty", func(t *testing.T) {
		body := "Model: opus\n\nDo something"
		dir, _, _ := ParseTemplate(body)
		assert.Equal(t, "", dir)
	})

	t.Run("missing Model returns empty", func(t *testing.T) {
		body := "Directory: /tmp\n\nDo something"
		_, model, _ := ParseTemplate(body)
		assert.Equal(t, "", model)
	})

	t.Run("removes forwarded message section", func(t *testing.T) {
		body := "Directory: /tmp\nModel: sonnet\n\nTask here\n---------- Forwarded message ----------\nOriginal content"
		dir, model, prompt := ParseTemplate(body)
		assert.Equal(t, "/tmp", dir)
		assert.Equal(t, "sonnet", model)
		assert.Equal(t, "Task here", prompt)
	})

	t.Run("removes quote prefixes", func(t *testing.T) {
		body := "> Directory: /tmp\n> Model: sonnet\n> \n> Do the thing"
		dir, model, prompt := ParseTemplate(body)
		assert.Equal(t, "/tmp", dir)
		assert.Equal(t, "sonnet", model)
		assert.Equal(t, "Do the thing", prompt)
	})

	t.Run("handles quote prefix without space", func(t *testing.T) {
		body := ">Directory: /tmp\n>Model: opus\n>\n>Build it"
		dir, model, prompt := ParseTemplate(body)
		assert.Equal(t, "/tmp", dir)
		assert.Equal(t, "opus", model)
		assert.Equal(t, "Build it", prompt)
	})

	t.Run("removes Gmail reply citation (Korean)", func(t *testing.T) {
		body := "Directory: ~/project\nModel: opus\n\n코드를 분석해줘\n\n2026년 2월 23일 (월) PM 12:30, <user@gmail.com>님이 작성:\n\nHow to create a new Claude Code session\nIMPORTANT — Do NOT change..."
		dir, model, prompt := ParseTemplate(body)
		assert.Equal(t, "~/project", dir)
		assert.Equal(t, "opus", model)
		assert.Equal(t, "코드를 분석해줘", prompt)
	})

	t.Run("removes Gmail reply citation (English)", func(t *testing.T) {
		body := "Directory: ~/work\nModel: sonnet\n\nAnalyze the code\n\nOn Mon, Feb 23, 2026 at 12:30 PM <user@gmail.com> wrote:\n\nOriginal template content..."
		dir, model, prompt := ParseTemplate(body)
		assert.Equal(t, "~/work", dir)
		assert.Equal(t, "sonnet", model)
		assert.Equal(t, "Analyze the code", prompt)
	})
}

func TestExtractTextFromHTML(t *testing.T) {
	t.Run("strips HTML tags", func(t *testing.T) {
		html := "<html><body><p>Hello <b>world</b></p><br/><p>Test</p></body></html>"
		text := ExtractTextFromHTML(html)
		assert.Contains(t, text, "Hello world")
		assert.Contains(t, text, "Test")
		assert.NotContains(t, text, "<")
	})

	t.Run("decodes HTML entities", func(t *testing.T) {
		html := "<p>A &amp; B &lt; C</p>"
		text := ExtractTextFromHTML(html)
		assert.Contains(t, text, "A & B < C")
	})

	t.Run("preserves line breaks from block elements", func(t *testing.T) {
		html := "<div>Line 1</div><div>Line 2</div>"
		text := ExtractTextFromHTML(html)
		assert.Contains(t, text, "Line 1")
		assert.Contains(t, text, "Line 2")
	})
}
