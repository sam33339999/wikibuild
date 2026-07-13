package llm

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	// Go regexp has no backrefs — strip each block type separately.
	reScript   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reNoscript = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHTMLTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reMultiSpace  = regexp.MustCompile(`[\t\r\f\v]+`)
	reManyNewlines = regexp.MustCompile(`\n{3,}`)
)

// PlainTextFromHTML strips tags/scripts and returns plain text clipped to MaxBodyBytes.
// Used for html_upload AI SEO so we send readable content, not raw markup.
func PlainTextFromHTML(html string) string {
	s := strings.TrimSpace(html)
	if s == "" {
		return ""
	}
	s = reScript.ReplaceAllString(s, " ")
	s = reStyle.ReplaceAllString(s, " ")
	s = reNoscript.ReplaceAllString(s, " ")
	s = reHTMLComment.ReplaceAllString(s, " ")
	s = reHTMLTag.ReplaceAllString(s, " ")
	// Common entities (minimal; enough for SEO blurbs).
	replacer := strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
	)
	s = replacer.Replace(s)
	s = reMultiSpace.ReplaceAllString(s, " ")
	// Normalize line breaks from block tags already turned into spaces; collapse.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	s = reManyNewlines.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= MaxBodyBytes {
		return s
	}
	n := MaxBodyBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
