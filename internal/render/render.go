// Package render converts Markdown to HTML using Goldmark with GFM, linkify
// and syntax highlighting, plus GitHub-style heading IDs and a generated
// table of contents. It is a pure package (no I/O, no DB) so it stays fully
// unit-testable as an L1 layer.
package render

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// md is the shared Goldmark instance: GFM (tables/strikethrough/tasklists),
// autolinked URLs, syntax-highlighted code, and raw-HTML escaping for safety.
// Heading IDs are assigned separately (see renderWithTOC) so the same pipeline
// serves both Render and RenderWithTOC.
//
// Code highlighting uses chroma CSS classes (prefix ch-), not monokai inline
// colors. Token colors live in static/css/chroma.css (light github / dark monokai
// under data-theme) so they match the site --code-bg instead of clashing.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Linkify,
		highlighting.NewHighlighting(
			// Style name is unused for colors when WithClasses is on; classes
			// are theme-agnostic token kinds. Keep a valid style for fallbacks.
			highlighting.WithStyle("github"),
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(true),
				chromahtml.ClassPrefix("ch-"),
			),
		),
	),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

// Heading is one entry in the table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// Render converts Markdown to HTML (with heading IDs). Code is syntax
// highlighted via chroma classes; colors come from static/css/chroma.css.
func Render(markdown string) string {
	htmlStr, _ := renderWithTOC(markdown)
	return htmlStr
}

// RenderWithTOC converts Markdown to HTML and returns the heading outline.
// Heading IDs in the HTML match the TOC IDs exactly, so anchors link back.
func RenderWithTOC(markdown string) (string, []Heading) {
	return renderWithTOC(markdown)
}

func renderWithTOC(markdown string) (string, []Heading) {
	// Translate [[wikilinks]] to markdown links before parsing.
	source := []byte(wikilinksToMarkdown(markdown))
	doc := md.Parser().Parse(text.NewReader(source))

	toc := assignHeadingIDs(doc, source)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		// Goldmark rendering only errors on writer failures; bytes.Buffer
		// never does. Surface as empty HTML rather than panicking.
		return "", toc
	}
	return buf.String(), toc
}

// assignHeadingIDs walks heading nodes, slugifies their text, deduplicates
// collisions GitHub-style (slug, slug-1, slug-2…), sets the id attribute so
// the HTML renderer emits it, and returns the collected outline.
func assignHeadingIDs(doc ast.Node, source []byte) []Heading {
	var toc []Heading
	seen := map[string]bool{}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		id := slugify(textOf(h, source))
		if id == "" {
			return ast.WalkContinue, nil // heading with no text: skip
		}
		id = dedupe(id, seen)
		h.SetAttribute([]byte("id"), []byte(id))
		toc = append(toc, Heading{Level: h.Level, ID: id, Text: textOf(h, source)})
		return ast.WalkContinue, nil
	})
	return toc
}

// dedupe returns a unique id for the given base, recording it as seen. Collisions
// get a -N suffix (1-based), matching GitHub's behaviour.
func dedupe(base string, seen map[string]bool) string {
	id := base
	n := 1
	for seen[id] {
		id = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	seen[id] = true
	return id
}

// textOf returns the concatenated text of a node's descendants, covering
// plain text and inline code so headings like "The `foo` flag" read correctly.
func textOf(n ast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.Kind() {
		case ast.KindText:
			b.Write(c.(*ast.Text).Text(source))
		case ast.KindCodeSpan:
			b.WriteString(textOf(c, source))
		case ast.KindLink:
			b.WriteString(textOf(c, source))
		default:
			b.WriteString(textOf(c, source))
		}
	}
	return b.String()
}

// slugify produces a GitHub-style anchor id: lowercase, whitespace to hyphens,
// punctuation dropped, runs of hyphens collapsed. Unicode letters/digits
// (including CJK) are preserved.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevDash = false
		case r == ' ' || r == '\t' || r == '-' || r == '_':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		default:
			// drop other punctuation
		}
	}
	return strings.Trim(b.String(), "-")
}
