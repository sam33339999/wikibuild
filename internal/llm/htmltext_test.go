package llm_test

import (
	"strings"
	"testing"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestPlainTextFromHTML_StripsTagsAndScripts(t *testing.T) {
	html := `<!doctype html><html><head><title>T</title>
<style>body{color:red}</style>
<script>alert(1)</script></head>
<body><h1>Hello</h1><p>World <b>bold</b> &amp; friends.</p>
<!-- comment --><p>More</p></body></html>`
	got := llm.PlainTextFromHTML(html)
	require.Contains(t, got, "Hello")
	require.Contains(t, got, "World")
	require.Contains(t, got, "bold")
	require.Contains(t, got, "friends")
	require.Contains(t, got, "More")
	require.NotContains(t, got, "alert")
	require.NotContains(t, got, "color:red")
	require.NotContains(t, got, "<")
	require.NotContains(t, got, "comment")
}

func TestPlainTextFromHTML_Empty(t *testing.T) {
	require.Empty(t, llm.PlainTextFromHTML(""))
	require.Empty(t, llm.PlainTextFromHTML("   <div></div>  "))
}

func TestPlainTextFromHTML_ClipsToMaxBody(t *testing.T) {
	// Many paragraphs so plain text exceeds MaxBodyBytes.
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 5000; i++ {
		b.WriteString("<p>段落內容足夠長一二三四五六七八九十</p>")
	}
	b.WriteString("</body></html>")
	got := llm.PlainTextFromHTML(b.String())
	require.LessOrEqual(t, len(got), llm.MaxBodyBytes)
	require.NotEmpty(t, got)
}
