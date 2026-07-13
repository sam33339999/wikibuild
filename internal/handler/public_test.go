package handler_test

import (
	"context"
	"fmt"
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

func publicApp(t *testing.T) (*fiber.App, *inmem.Store) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewPublic(repo)
	app := fiber.New()
	app.Get("/", h.Index)
	app.Get("/:slug", h.Article)
	return app, repo
}

func seedArticle(t *testing.T, repo *inmem.Store, slug, title, body string, status model.Status, vis model.Visibility) model.Article {
	t.Helper()
	pub := time.Unix(1_700_000_000, 0)
	a := model.Article{
		Slug: slug, Title: title, Body: body,
		Type: model.ArticleTypeMarkdown, Status: status, Visibility: vis,
	}
	if status == model.StatusPublished {
		a.PublishedAt = &pub
	}
	created, err := repo.CreateArticle(context.Background(), a)
	require.NoError(t, err)
	return created
}

func TestPublic_Index_ShowsOnlyPublishedPublic(t *testing.T) {
	app, repo := publicApp(t)
	seedArticle(t, repo, "pub", "Published", "x", model.StatusPublished, model.VisibilityPublic)
	seedArticle(t, repo, "draft", "Draft", "x", model.StatusDraft, model.VisibilityPublic)
	seedArticle(t, repo, "priv", "Private", "x", model.StatusPublished, model.VisibilityPrivate)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Published")
	require.NotContains(t, string(body), "Draft")
	require.NotContains(t, string(body), "Private")
}

func TestPublic_Index_Pagination(t *testing.T) {
	app, repo := publicApp(t)
	// Zero-padded titles avoid substring collisions ("Post 01" vs "Post 11").
	for i := 0; i < 12; i++ {
		seedArticle(t, repo, "p"+itoa(i), fmt.Sprintf("Post %02d", i), "x",
			model.StatusPublished, model.VisibilityPublic)
	}

	// Store returns newest-first, so page 1 = Post 11..Post 02.
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/?page=1", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Post 11")
	require.Contains(t, string(body), "Post 02")
	require.NotContains(t, string(body), "Post 01", "page 1 caps at page size")
	require.NotContains(t, string(body), "Post 00")

	// Page 2 = Post 01, Post 00.
	resp2, _ := app.Test(httptest.NewRequest(http.MethodGet, "/?page=2", nil))
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, _ := io.ReadAll(resp2.Body)
	require.Contains(t, string(body2), "Post 01")
	require.Contains(t, string(body2), "Post 00")
	require.NotContains(t, string(body2), "Post 11")

	// Page 3: empty (but still 200).
	resp3, _ := app.Test(httptest.NewRequest(http.MethodGet, "/?page=3", nil))
	require.Equal(t, http.StatusOK, resp3.StatusCode)
}

func TestPublic_Article_RendersMarkdown(t *testing.T) {
	app, repo := publicApp(t)
	seedArticle(t, repo, "hello", "Hello", "# Hello\n\nA **bold** word.", model.StatusPublished, model.VisibilityPublic)

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "<h1")
	require.Contains(t, string(body), "<strong>bold</strong>")
}

func TestPublic_Article_TOCRendered(t *testing.T) {
	app, repo := publicApp(t)
	seedArticle(t, repo, "toc", "TOC", "# Intro\n\n## Details\n\ntext", model.StatusPublished, model.VisibilityPublic)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/toc", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "intro")
	require.Contains(t, string(body), "details")
}

func TestPublic_Article_NotFound(t *testing.T) {
	app, _ := publicApp(t)
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/nope", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPublic_Article_DraftIs404(t *testing.T) {
	app, repo := publicApp(t)
	seedArticle(t, repo, "draft", "Draft", "x", model.StatusDraft, model.VisibilityPublic)
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/draft", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPublic_Article_NonPublicVisibilityIs404(t *testing.T) {
	app, repo := publicApp(t)
	seedArticle(t, repo, "priv", "Private", "x", model.StatusPublished, model.VisibilityPrivate)
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/priv", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return string(b)
}
