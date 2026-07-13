package render

import (
	"regexp"
	"strings"
)

// wikilinkRe matches [[slug]] or [[display|slug]]. The inner content excludes
// ']' so a run of brackets closes at the first "]]".
var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// wikilinksToMarkdown converts [[...]] wiki links to standard markdown links
// so Goldmark renders them. Forms:
//
//	[[slug]]         -> [slug](/slug)
//	[[display|slug]] -> [display](/slug)
//
// Applied to the source before parsing, so wikilinks work everywhere
// (paragraphs, headings, lists). Slugs are used as-is (no title lookup), so
// the display|slug form covers human-friendly labels.
func wikilinksToMarkdown(body string) string {
	return wikilinkRe.ReplaceAllStringFunc(body, func(m string) string {
		inner := m[2 : len(m)-2] // strip [[ ]]
		display, slug, found := strings.Cut(inner, "|")
		display = strings.TrimSpace(display)
		slug = strings.TrimSpace(slug)
		if display == "" {
			display = slug
		}
		if !found || slug == "" {
			// [[display]] with no pipe: slug == display.
			slug = display
		}
		// Escape any "]" in the display so the markdown link stays well-formed.
		return "[" + strings.ReplaceAll(display, "]", "\\]") + "](/" + slug + ")"
	})
}
