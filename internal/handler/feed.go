package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/feed"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/sitebrand"
	"github.com/sam33339999/wikibuild/internal/store"
)

// Syndication serves RSS / Atom / JSON Feed, sitemap.xml, and robots.txt.
type Syndication struct {
	repo    store.Repository
	baseURL string
	title   string
}

// NewSyndication builds a Syndication handler. baseURL has no trailing slash.
func NewSyndication(repo store.Repository, baseURL, title string) *Syndication {
	if title == "" {
		title = "WikiBuild"
	}
	return &Syndication{repo: repo, baseURL: baseURL, title: title}
}

func (h *Syndication) site(c fiber.Ctx) feed.Site {
	b := sitebrand.Load(c.Context(), h.repo, h.title)
	desc := b.Tagline
	if desc == "" {
		desc = b.Name
	}
	return feed.Site{
		Title:       b.Name,
		BaseURL:     h.baseURL,
		Description: desc,
	}
}

func (h *Syndication) publicItems(c fiber.Ctx) ([]feed.Item, error) {
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
	})
	if err != nil {
		return nil, err
	}
	return feed.FromArticles(items), nil
}

// RSS serves application/rss+xml at /feed.
func (h *Syndication) RSS(c fiber.Ctx) error {
	items, err := h.publicItems(c)
	if err != nil {
		return err
	}
	c.Set("Content-Type", "application/rss+xml; charset=utf-8")
	return c.SendString(feed.RSS(h.site(c), items))
}

// Atom serves application/atom+xml at /feed/atom.
func (h *Syndication) Atom(c fiber.Ctx) error {
	items, err := h.publicItems(c)
	if err != nil {
		return err
	}
	c.Set("Content-Type", "application/atom+xml; charset=utf-8")
	return c.SendString(feed.Atom(h.site(c), items))
}

// JSONFeed serves application/feed+json at /feed.json.
func (h *Syndication) JSONFeed(c fiber.Ctx) error {
	items, err := h.publicItems(c)
	if err != nil {
		return err
	}
	c.Set("Content-Type", "application/feed+json; charset=utf-8")
	return c.SendString(feed.JSON(h.site(c), items))
}

// Sitemap serves /sitemap.xml for published public articles.
func (h *Syndication) Sitemap(c fiber.Ctx) error {
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
	})
	if err != nil {
		return err
	}
	slugs := make([]string, 0, len(items))
	for _, a := range items {
		slugs = append(slugs, a.Slug)
	}
	c.Set("Content-Type", "application/xml; charset=utf-8")
	return c.SendString(feed.Sitemap(h.baseURL, slugs))
}

// Robots serves /robots.txt.
func (h *Syndication) Robots(c fiber.Ctx) error {
	c.Set("Content-Type", "text/plain; charset=utf-8")
	return c.SendString(feed.Robots(h.baseURL))
}
