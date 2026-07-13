package feed_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/feed"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/stretchr/testify/require"
)

func sampleItems() (feed.Site, []feed.Item) {
	pub := time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)
	site := feed.Site{Title: "My Wiki", BaseURL: "https://ex.com", Description: "notes"}
	items := feed.FromArticles([]model.Article{{
		Slug: "hello", Title: "Hello & Co", Body: "First **paragraph** of content.",
		PublishedAt: &pub,
	}})
	return site, items
}

func TestRSS_ContainsItem(t *testing.T) {
	site, items := sampleItems()
	out := feed.RSS(site, items)
	require.Contains(t, out, `<rss version="2.0">`)
	require.Contains(t, out, "<title>My Wiki</title>")
	require.Contains(t, out, "https://ex.com/hello")
	require.Contains(t, out, "Hello &amp; Co")
	require.Contains(t, out, "<pubDate>")
}

func TestAtom_ContainsEntry(t *testing.T) {
	site, items := sampleItems()
	out := feed.Atom(site, items)
	require.Contains(t, out, `xmlns="http://www.w3.org/2005/Atom"`)
	require.Contains(t, out, "<entry>")
	require.Contains(t, out, "https://ex.com/hello")
	require.Contains(t, out, `rel="self"`)
}

func TestJSON_ValidStructure(t *testing.T) {
	site, items := sampleItems()
	out := feed.JSON(site, items)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	require.Equal(t, "https://jsonfeed.org/version/1.1", m["version"])
	require.Equal(t, "My Wiki", m["title"])
	arr, ok := m["items"].([]any)
	require.True(t, ok)
	require.Len(t, arr, 1)
}

func TestSitemap_AndRobots(t *testing.T) {
	sm := feed.Sitemap("https://ex.com", []string{"a", "b"})
	require.Contains(t, sm, "https://ex.com/")
	require.Contains(t, sm, "https://ex.com/a")
	require.Contains(t, sm, "https://ex.com/b")

	rb := feed.Robots("https://ex.com")
	require.True(t, strings.HasPrefix(rb, "User-agent: *"))
	require.Contains(t, rb, "Sitemap: https://ex.com/sitemap.xml")
}

func TestFromArticles_SummaryStripsMarkdownNoise(t *testing.T) {
	items := feed.FromArticles([]model.Article{{
		Slug: "x", Title: "T", Body: "# Heading\n\nHello **world**",
	}})
	require.NotContains(t, items[0].Summary, "#")
	require.Contains(t, items[0].Summary, "Hello")
}

func TestFromArticles_PrefersAuthorSummary(t *testing.T) {
	items := feed.FromArticles([]model.Article{{
		Slug: "x", Title: "T", Body: "long body text that should not appear",
		Summary: "Author blurb", MetaDescription: "meta only",
	}})
	require.Equal(t, "Author blurb", items[0].Summary)
}
