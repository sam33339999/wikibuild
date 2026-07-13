package seo_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/seo"
	"github.com/stretchr/testify/require"
)

func TestEffectiveTitle(t *testing.T) {
	require.Equal(t, "Display", seo.EffectiveTitle(model.Article{Title: "Display"}))
	require.Equal(t, "SEO", seo.EffectiveTitle(model.Article{Title: "Display", SEOTitle: "SEO"}))
	require.Equal(t, "Display", seo.EffectiveTitle(model.Article{Title: "Display", SEOTitle: "  "}))
}

func TestEffectiveMetaDescription_FallbackMatrix(t *testing.T) {
	a := model.Article{
		Body:            "# Hello\n\nThis is the **body** with a [link](https://x.com) and 中文內容足夠長。",
		Summary:         "Human summary",
		MetaDescription: "Meta desc",
	}
	require.Equal(t, "Meta desc", seo.EffectiveMetaDescription(a))

	a.MetaDescription = ""
	require.Equal(t, "Human summary", seo.EffectiveMetaDescription(a))

	a.Summary = ""
	got := seo.EffectiveMetaDescription(a)
	require.NotContains(t, got, "https://")
	require.NotContains(t, got, "**")
	require.Contains(t, got, "Hello")
	require.Contains(t, got, "body")
	require.Contains(t, got, "中文")
}

func TestEffectiveFeedSummary_PrefersSummary(t *testing.T) {
	a := model.Article{
		Body:            "body only",
		Summary:         "feed sum",
		MetaDescription: "meta only",
	}
	require.Equal(t, "feed sum", seo.EffectiveFeedSummary(a))
	a.Summary = ""
	require.Equal(t, "meta only", seo.EffectiveFeedSummary(a))
}

func TestEffectiveOGImage(t *testing.T) {
	require.Equal(t, "", seo.EffectiveOGImage(model.Article{}))
	require.Equal(t, "/media/cover.png", seo.EffectiveOGImage(model.Article{CoverImageURL: "/media/cover.png"}))
	require.Equal(t, "/media/og.png", seo.EffectiveOGImage(model.Article{
		CoverImageURL: "/media/cover.png",
		OGImageURL:    "/media/og.png",
	}))
}

func TestClipPlain_RuneSafeNotBytes(t *testing.T) {
	// 200 CJK runes — old byte-based clip would mid-character truncate UTF-8.
	body := strings.Repeat("測", 200)
	got := seo.ClipPlain(body, 155)
	require.True(t, strings.HasSuffix(got, "…"))
	require.Equal(t, 155, utf8.RuneCountInString(strings.TrimSuffix(got, "…")))
	require.True(t, utf8.ValidString(got))
}

func TestClipPlain_StripsCodeFenceAndLinks(t *testing.T) {
	body := "Intro\n\n```go\nfmt.Println(\"hi\")\n```\n\nSee [docs](https://example.com/path) and [[wikilink]]."
	got := seo.ClipPlain(body, 500)
	require.NotContains(t, got, "fmt.Println")
	require.NotContains(t, got, "https://")
	require.Contains(t, got, "Intro")
	require.Contains(t, got, "docs")
	require.Contains(t, got, "wikilink")
}
