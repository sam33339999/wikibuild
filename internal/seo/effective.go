package seo

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sam33339999/wikibuild/internal/model"
)

// MetaDescriptionMaxRunes is the soft cap for auto-clipped meta descriptions
// (~search-snippet length; rune-safe for CJK).
const MetaDescriptionMaxRunes = 155

// FeedSummaryMaxRunes is the soft cap for feed item summaries when clipping body.
const FeedSummaryMaxRunes = 280

var (
	reCodeFence = regexp.MustCompile("(?s)```.*?```")
	reInlineCode = regexp.MustCompile("`[^`]*`")
	reMDLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reMDImage   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reHTMLTag   = regexp.MustCompile(`(?s)<[^>]+>`)
	reWikiLink  = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)
	reMDHeading = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reMDBold    = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	reMDItalic  = regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_`)
)

// EffectiveTitle returns seo_title if set, otherwise the display title.
func EffectiveTitle(a model.Article) string {
	if s := strings.TrimSpace(a.SEOTitle); s != "" {
		return s
	}
	return a.Title
}

// EffectiveMetaDescription: meta_description → summary → body clip.
func EffectiveMetaDescription(a model.Article) string {
	if s := strings.TrimSpace(a.MetaDescription); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.Summary); s != "" {
		return s
	}
	return ClipPlain(a.Body, MetaDescriptionMaxRunes)
}

// EffectiveFeedSummary: summary → meta_description → body clip (longer).
func EffectiveFeedSummary(a model.Article) string {
	if s := strings.TrimSpace(a.Summary); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.MetaDescription); s != "" {
		return s
	}
	return ClipPlain(a.Body, FeedSummaryMaxRunes)
}

// EffectiveOGImage: og_image_url → cover_image_url → empty.
func EffectiveOGImage(a model.Article) string {
	if s := strings.TrimSpace(a.OGImageURL); s != "" {
		return s
	}
	return strings.TrimSpace(a.CoverImageURL)
}

// ClipPlain strips common Markdown/HTML noise and truncates to max runes.
// Empty body yields empty string.
func ClipPlain(body string, maxRunes int) string {
	s := stripMarkup(body)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	if s == "" || maxRunes <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

func stripMarkup(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = reCodeFence.ReplaceAllString(s, " ")
	s = reInlineCode.ReplaceAllString(s, " ")
	s = reMDImage.ReplaceAllString(s, " ")
	s = reMDLink.ReplaceAllString(s, "$1")
	s = reWikiLink.ReplaceAllString(s, "$1")
	s = reHTMLTag.ReplaceAllString(s, " ")
	s = reMDHeading.ReplaceAllString(s, "")
	s = reMDBold.ReplaceAllString(s, "$1$2")
	s = reMDItalic.ReplaceAllString(s, "$1$2")
	// Remaining single-char markdown noise.
	s = strings.ReplaceAll(s, "#", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ">", " ")
	return s
}
