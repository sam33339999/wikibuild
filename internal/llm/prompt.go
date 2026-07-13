package llm

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ClipBody returns body trimmed and truncated to MaxBodyBytes (UTF-8 safe).
func ClipBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) <= MaxBodyBytes {
		return body
	}
	// Walk back to a rune boundary.
	n := MaxBodyBytes
	for n > 0 && !utf8.RuneStart(body[n]) {
		n--
	}
	return body[:n]
}

// BuildSEOMessages builds system + user messages for SEO generation.
func BuildSEOMessages(title, body string) []Message {
	body = ClipBody(body)
	system := strings.TrimSpace(`You are an SEO assistant for a personal blog/wiki.
Given an article title and markdown body, produce a single JSON object only (no markdown fences, no commentary) with exactly these keys:
- "outline": array of short bullet strings (3–8 items) summarizing structure or key points
- "meta_description": plain text, about 120–155 characters, no markdown, suitable for HTML meta description
- "summary": 2–4 plain sentences for humans and RSS feeds

Match the language of the article body (prefer Traditional Chinese if the body is Chinese).
Do not invent facts not supported by the body.`)

	user := fmt.Sprintf("TITLE: %s\n\nBODY:\n%s", strings.TrimSpace(title), body)
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}
