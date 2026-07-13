package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func feedApp(t *testing.T) (*fiber.App, *inmem.Store) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewSyndication(repo, "https://ex.com", "Test Site")
	app := fiber.New()
	app.Get("/feed", h.RSS)
	app.Get("/feed/atom", h.Atom)
	app.Get("/feed.json", h.JSONFeed)
	app.Get("/sitemap.xml", h.Sitemap)
	app.Get("/robots.txt", h.Robots)
	return app, repo
}

func seedPublic(t *testing.T, repo *inmem.Store, slug, title string) {
	t.Helper()
	pub := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: slug, Title: title, Body: "hello body",
		Type: model.ArticleTypeMarkdown, Status: model.StatusPublished,
		Visibility: model.VisibilityPublic, PublishedAt: &pub,
	})
	require.NoError(t, err)
}

func TestSyndication_RSS(t *testing.T) {
	app, repo := feedApp(t)
	seedPublic(t, repo, "hello", "Hello Post")
	// draft must not appear
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "draft", Title: "Draft", Body: "x", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/feed", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "rss+xml")
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Hello Post")
	require.Contains(t, string(body), "https://ex.com/hello")
	require.NotContains(t, string(body), "Draft")
}

func TestSyndication_AtomAndJSON(t *testing.T) {
	app, repo := feedApp(t)
	seedPublic(t, repo, "a", "A")

	for _, path := range []string{"/feed/atom", "/feed.json"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		body, _ := io.ReadAll(resp.Body)
		require.Contains(t, string(body), "A", path)
	}
}

func TestSyndication_SitemapAndRobots(t *testing.T) {
	app, repo := feedApp(t)
	seedPublic(t, repo, "p1", "P1")

	sm, err := app.Test(httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(sm.Body)
	require.Contains(t, string(body), "https://ex.com/p1")

	rb, err := app.Test(httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	require.NoError(t, err)
	body, _ = io.ReadAll(rb.Body)
	require.Contains(t, string(body), "Sitemap: https://ex.com/sitemap.xml")
}
