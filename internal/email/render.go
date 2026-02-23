package email

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape codes from terminal output.
func StripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

var md = goldmark.New(
	goldmark.WithExtensions(
		highlighting.NewHighlighting(
			highlighting.WithStyle("github"),
		),
	),
)

const htmlTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;line-height:1.6;color:#333;max-width:800px;margin:0 auto;padding:20px;">
%s
</body>
</html>`

// RenderHTML converts Markdown text to a complete HTML email document.
func RenderHTML(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("markdown conversion failed: %w", err)
	}
	return fmt.Sprintf(htmlTemplate, buf.String()), nil
}
