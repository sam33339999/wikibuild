package render_test

import (
	"testing"

	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/stretchr/testify/require"
)

func TestWikilinks_SimpleSlug(t *testing.T) {
	out := render.Render("See [[hello-world]] here.")
	require.Contains(t, out, `<a href="/hello-world">hello-world</a>`)
}

func TestWikilinks_DisplayPipe(t *testing.T) {
	out := render.Render("See [[Display Text|hello-world]] here.")
	require.Contains(t, out, `<a href="/hello-world">Display Text</a>`)
}

func TestWikilinks_MultipleInOneLine(t *testing.T) {
	out := render.Render("[[a]] and [[B text|b]] link.")
	require.Contains(t, out, `<a href="/a">a</a>`)
	require.Contains(t, out, `<a href="/b">B text</a>`)
}

func TestWikilinks_AcrossParagraphs(t *testing.T) {
	out := render.Render("# Title\n\nFirst [[one]] paragraph.\n\nSecond [[two|two-slug]] paragraph.\n")
	require.Contains(t, out, `<a href="/one">one</a>`)
	require.Contains(t, out, `<a href="/two-slug">two</a>`)
}

func TestWikilinks_NoMatchForNormalBrackets(t *testing.T) {
	// Single brackets are not wikilinks.
	out := render.Render("array[int] is fine")
	require.NotContains(t, out, `<a href="/int">`)
}

func TestWikilinks_PreservesSurroundingMarkdown(t *testing.T) {
	out := render.Render("This is **bold [[slug]]** text.")
	require.Contains(t, out, `<a href="/slug">slug</a>`)
	require.Contains(t, out, "<strong>")
}
