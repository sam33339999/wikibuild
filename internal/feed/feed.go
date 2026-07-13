// Package feed builds syndication documents (RSS 2.0, Atom, JSON Feed) and
// sitemap/robots text from article metadata. Pure string builders — no I/O.
package feed

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/seo"
)

// Site describes the feed channel / sitemap origin.
type Site struct {
	Title       string
	BaseURL     string // no trailing slash, e.g. https://example.com
	Description string
}

// Item is one feed entry derived from a published public article.
type Item struct {
	Title   string
	Slug    string
	Summary string
	Updated time.Time
}

// FromArticles maps domain articles into feed items (caller filters visibility).
// Title uses display title (feeds show human title; SEO title is for HTML meta).
// Summary prefers author summary / meta / body clip (see seo.EffectiveFeedSummary).
func FromArticles(articles []model.Article) []Item {
	out := make([]Item, 0, len(articles))
	for _, a := range articles {
		it := Item{
			Title:   a.Title,
			Slug:    a.Slug,
			Summary: seo.EffectiveFeedSummary(a),
		}
		if a.PublishedAt != nil {
			it.Updated = a.PublishedAt.UTC()
		} else {
			it.Updated = a.UpdatedAt.UTC()
		}
		out = append(out, it)
	}
	return out
}

func abs(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// RSS builds an RSS 2.0 document.
func RSS(site Site, items []Item) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0"><channel>`)
	b.WriteString("<title>" + xmlEsc(site.Title) + "</title>")
	b.WriteString("<link>" + xmlEsc(site.BaseURL) + "</link>")
	b.WriteString("<description>" + xmlEsc(site.Description) + "</description>")
	for _, it := range items {
		link := abs(site.BaseURL, "/"+it.Slug)
		b.WriteString("<item>")
		b.WriteString("<title>" + xmlEsc(it.Title) + "</title>")
		b.WriteString("<link>" + xmlEsc(link) + "</link>")
		b.WriteString("<guid isPermaLink=\"true\">" + xmlEsc(link) + "</guid>")
		if !it.Updated.IsZero() {
			b.WriteString("<pubDate>" + it.Updated.Format(time.RFC1123Z) + "</pubDate>")
		}
		if it.Summary != "" {
			b.WriteString("<description>" + xmlEsc(it.Summary) + "</description>")
		}
		b.WriteString("</item>")
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

// Atom builds an Atom 1.0 feed.
func Atom(site Site, items []Item) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom">`)
	b.WriteString("<title>" + xmlEsc(site.Title) + "</title>")
	b.WriteString(`<link href="` + xmlEsc(site.BaseURL) + `" rel="alternate"/>`)
	b.WriteString(`<link href="` + xmlEsc(abs(site.BaseURL, "/feed/atom")) + `" rel="self"/>`)
	b.WriteString("<id>" + xmlEsc(site.BaseURL+"/") + "</id>")
	updated := time.Time{}
	for _, it := range items {
		if it.Updated.After(updated) {
			updated = it.Updated
		}
	}
	if !updated.IsZero() {
		b.WriteString("<updated>" + updated.Format(time.RFC3339) + "</updated>")
	}
	for _, it := range items {
		link := abs(site.BaseURL, "/"+it.Slug)
		b.WriteString("<entry>")
		b.WriteString("<title>" + xmlEsc(it.Title) + "</title>")
		b.WriteString(`<link href="` + xmlEsc(link) + `"/>`)
		b.WriteString("<id>" + xmlEsc(link) + "</id>")
		if !it.Updated.IsZero() {
			b.WriteString("<updated>" + it.Updated.Format(time.RFC3339) + "</updated>")
		}
		if it.Summary != "" {
			b.WriteString("<summary>" + xmlEsc(it.Summary) + "</summary>")
		}
		b.WriteString("</entry>")
	}
	b.WriteString(`</feed>`)
	return b.String()
}

// JSON builds a JSON Feed 1.1 document.
func JSON(site Site, items []Item) string {
	type jItem struct {
		ID            string `json:"id"`
		URL           string `json:"url"`
		Title         string `json:"title"`
		Summary       string `json:"summary,omitempty"`
		DatePublished string `json:"date_published,omitempty"`
	}
	type jFeed struct {
		Version     string  `json:"version"`
		Title       string  `json:"title"`
		HomePageURL string  `json:"home_page_url"`
		FeedURL     string  `json:"feed_url"`
		Description string  `json:"description,omitempty"`
		Items       []jItem `json:"items"`
	}
	jf := jFeed{
		Version:     "https://jsonfeed.org/version/1.1",
		Title:       site.Title,
		HomePageURL: site.BaseURL + "/",
		FeedURL:     abs(site.BaseURL, "/feed.json"),
		Description: site.Description,
		Items:       make([]jItem, 0, len(items)),
	}
	for _, it := range items {
		link := abs(site.BaseURL, "/"+it.Slug)
		ji := jItem{ID: link, URL: link, Title: it.Title, Summary: it.Summary}
		if !it.Updated.IsZero() {
			ji.DatePublished = it.Updated.Format(time.RFC3339)
		}
		jf.Items = append(jf.Items, ji)
	}
	raw, err := json.Marshal(jf)
	if err != nil {
		return `{"version":"https://jsonfeed.org/version/1.1","title":"","items":[]}`
	}
	return string(raw)
}

// Sitemap builds a basic urlset for the homepage and each article slug.
func Sitemap(baseURL string, slugs []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	b.WriteString("<url><loc>" + xmlEsc(abs(baseURL, "/")) + "</loc></url>")
	for _, slug := range slugs {
		b.WriteString("<url><loc>" + xmlEsc(abs(baseURL, "/"+slug)) + "</loc></url>")
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// Robots builds a robots.txt that allows all and points at the sitemap.
func Robots(baseURL string) string {
	return fmt.Sprintf("User-agent: *\nAllow: /\nSitemap: %s\n", abs(baseURL, "/sitemap.xml"))
}

func xmlEsc(s string) string {
	return html.EscapeString(s)
}
