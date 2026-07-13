package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func tagsApp(t *testing.T) (*fiber.App, store.Repository) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewTags(repo)
	app := fiber.New()
	app.Get("/admin/tags", h.List)
	app.Post("/admin/tags/rename", h.Rename)
	app.Post("/admin/tags/merge", h.Merge)
	return app, repo
}

func seedTagArticle(t *testing.T, repo store.Repository, slug string, tags ...string) {
	t.Helper()
	_, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: slug, Title: slug, Type: model.ArticleTypeMarkdown,
		Status: model.StatusPublished, Visibility: model.VisibilityPublic,
		Tags: tags,
	})
	require.NoError(t, err)
}

func TestTags_List_ShowsTagsAndCounts(t *testing.T) {
	app, repo := tagsApp(t)
	seedTagArticle(t, repo, "a", "go", "wiki")
	seedTagArticle(t, repo, "b", "go")

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/tags", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, "go")
	require.Contains(t, s, "wiki")
	require.Contains(t, s, "2") // go count
	require.Contains(t, s, "標籤")
}

func TestTags_Rename_UpdatesArticles(t *testing.T) {
	app, repo := tagsApp(t)
	seedTagArticle(t, repo, "a", "old", "keep")
	seedTagArticle(t, repo, "b", "old")

	form := url.Values{"from": {"old"}, "to": {"new"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/tags/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/admin/tags", resp.Header.Get("Location"))

	a, _ := repo.GetArticleBySlug(context.Background(), "a")
	require.ElementsMatch(t, []string{"new", "keep"}, a.Tags)
	b, _ := repo.GetArticleBySlug(context.Background(), "b")
	require.Equal(t, []string{"new"}, b.Tags)
}

func TestTags_Merge_Dedupes(t *testing.T) {
	app, repo := tagsApp(t)
	seedTagArticle(t, repo, "a", "draft", "wip")
	seedTagArticle(t, repo, "b", "draft", "done") // merge draft→done on b

	form := url.Values{"from": {"draft"}, "into": {"done"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/tags/merge", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	a, _ := repo.GetArticleBySlug(context.Background(), "a")
	require.ElementsMatch(t, []string{"done", "wip"}, a.Tags)
	b, _ := repo.GetArticleBySlug(context.Background(), "b")
	require.ElementsMatch(t, []string{"done"}, b.Tags)
}

func TestTags_Rename_EmptyRejected(t *testing.T) {
	app, _ := tagsApp(t)
	form := url.Values{"from": {""}, "to": {"x"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/tags/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
