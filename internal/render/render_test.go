package render_test

import (
	"strings"
	"testing"

	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/stretchr/testify/require"
)

func TestRender_BasicMarkdown(t *testing.T) {
	out := render.Render("# Title\n\nA paragraph with **bold**.")
	require.Contains(t, out, "<h1")
	require.Contains(t, out, "Title")
	require.Contains(t, out, "<strong>bold</strong>")
}

func TestRender_GFMTable(t *testing.T) {
	md := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	out := render.Render(md)
	require.Contains(t, out, "<table>")
	require.Contains(t, out, "<th>a</th>")
}

func TestRender_CodeBlock_Highlighted(t *testing.T) {
	md := "```go\nfunc main() {}\n```\n"
	out := render.Render(md)
	// Chroma highlights with inline color styles (M1); M7 switches to
	// classes + theme CSS for dark/light switching.
	require.Contains(t, out, "<pre")
	require.Contains(t, out, `style="color:`, "highlighted code should carry chroma color styles")
}

func TestRender_FencedCodeWithoutLanguage_StillPre(t *testing.T) {
	out := render.Render("```\nplain\n```\n")
	require.Contains(t, out, "<pre")
	require.Contains(t, out, "plain")
}

func TestRender_HeadingIDs(t *testing.T) {
	out := render.Render("# Hello World\n\n## Section Two\n")
	require.Contains(t, out, `<h1 id="hello-world"`, "h1 gets a slugified id")
	require.Contains(t, out, `<h2 id="section-two"`, "h2 gets a slugified id")
}

func TestRenderWithTOC_ExtractsHeadings(t *testing.T) {
	md := "# Intro\n\nbody\n\n## Details\n\n### Sub\n\n# Conclusion\n"
	html, toc := render.RenderWithTOC(md)

	require.Contains(t, html, `<h1 id="intro"`)
	require.Len(t, toc, 4)

	require.Equal(t, 1, toc[0].Level)
	require.Equal(t, "intro", toc[0].ID)
	require.Equal(t, "Intro", toc[0].Text)

	require.Equal(t, 2, toc[1].Level)
	require.Equal(t, "details", toc[1].ID)

	require.Equal(t, 3, toc[2].Level)
	require.Equal(t, "sub", toc[2].ID)

	require.Equal(t, 1, toc[3].Level)
	require.Equal(t, "conclusion", toc[3].ID)
}

func TestRenderWithTOC_NoHeadings(t *testing.T) {
	_, toc := render.RenderWithTOC("just a paragraph")
	require.Empty(t, toc)
}

func TestRenderWithTOC_DuplicateHeadings_GetUniqueIDs(t *testing.T) {
	md := "# Section\n\n# Section\n\n# Section\n"
	_, toc := render.RenderWithTOC(md)
	require.Len(t, toc, 3)
	require.Equal(t, "section", toc[0].ID)
	require.Equal(t, "section-1", toc[1].ID)
	require.Equal(t, "section-2", toc[2].ID)

	// The rendered HTML ids must match the TOC ids.
	html, _ := render.RenderWithTOC(md)
	require.Contains(t, html, `id="section"`)
	require.Contains(t, html, `id="section-1"`)
	require.Contains(t, html, `id="section-2"`)
}

func TestRender_EscapesHTML(t *testing.T) {
	out := render.Render("a <script>alert(1)</script> b")
	require.False(t, strings.Contains(out, "<script>alert"), "raw html must be escaped")
}

func TestRender_Linkify(t *testing.T) {
	out := render.Render("see https://example.com here")
	require.Contains(t, out, `<a href="https://example.com"`, "bare URLs become links")
}
