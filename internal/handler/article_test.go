package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

// articleApp wires an ArticleAdmin handler onto a fresh app with an inmem
// store. No CSRF middleware (handler-level tests bypass it, like the auth
// tests); the assembled-app CSRF path is covered by server tests.
func articleApp(t *testing.T) (*fiber.App, store.Repository) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewArticleAdmin(repo, fakeHasher{}, nil)
	app := fiber.New()
	app.Get("/admin", h.List)
	app.Get("/admin/new", h.NewForm)
	app.Post("/admin/new", h.Create)
	app.Get("/admin/:id/edit", h.EditForm)
	app.Post("/admin/:id", h.Update)
	app.Post("/admin/:id/delete", h.Delete)
	return app, repo
}

func postArticle(app *fiber.App, path string, form url.Values) *http.Response {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := app.Test(req)
	return resp
}

func articleForm() url.Values {
	f := url.Values{}
	f.Set("slug", "hello-world")
	f.Set("title", "Hello World")
	f.Set("body", "# Hello\n\nSome text.")
	f.Set("tags", "go, web")
	f.Set("status", "draft")
	f.Set("visibility", "public")
	return f
}

func TestArticleAdmin_Create(t *testing.T) {
	app, repo := articleApp(t)
	resp := postArticle(app, "/admin/new", articleForm())

	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	loc := resp.Header.Get("Location")
	require.True(t, strings.HasPrefix(loc, "/admin/") && strings.HasSuffix(loc, "/edit"),
		"create should redirect to the edit page, got %q", loc)

	items, _, err := repo.ListArticles(context.Background(), store.ListQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "hello-world", items[0].Slug)
	require.Equal(t, model.ArticleTypeMarkdown, items[0].Type)
	require.Equal(t, []string{"go", "web"}, items[0].Tags)
}

func TestArticleAdmin_List_SearchQuery(t *testing.T) {
	app, repo := articleApp(t)
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "go-post", Title: "Learning Go", Body: "intro", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "rust-post", Title: "Rust Book", Body: "ownership", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin?q=go", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, "Learning Go")
	require.NotContains(t, s, "Rust Book")
	require.Contains(t, s, `name="q"`) // search form present
}

func TestArticleAdmin_Create_DuplicateSlug(t *testing.T) {
	app, _ := articleApp(t)
	postArticle(app, "/admin/new", articleForm())

	resp := postArticle(app, "/admin/new", articleForm())
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestArticleAdmin_Create_EmptySlug(t *testing.T) {
	app, _ := articleApp(t)
	f := articleForm()
	f.Set("slug", "")
	resp := postArticle(app, "/admin/new", f)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestArticleAdmin_List_ShowsTitles(t *testing.T) {
	app, repo := articleApp(t)
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "a", Title: "Alpha Post", Type: model.ArticleTypeMarkdown,
		Status: model.StatusPublished, Visibility: model.VisibilityPublic,
	})
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "b", Title: "Beta Post", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Alpha Post")
	require.Contains(t, string(body), "Beta Post")
}

func TestArticleAdmin_NewForm(t *testing.T) {
	app, _ := articleApp(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "<form")
}

func TestArticleAdmin_EditForm_PreFilled(t *testing.T) {
	app, repo := articleApp(t)
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "edit-me", Title: "Edit Me", Body: "original body",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft,
		Visibility: model.VisibilityPublic, Tags: []string{"x"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin/"+strconv.FormatInt(a.ID, 10)+"/edit", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Edit Me")
	require.Contains(t, string(body), "original body")
}

func TestArticleAdmin_EditForm_NotFound(t *testing.T) {
	app, _ := articleApp(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/9999/edit", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestArticleAdmin_Update(t *testing.T) {
	app, repo := articleApp(t)
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "old-slug", Title: "Old", Body: "old",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft,
		Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)

	f := url.Values{}
	f.Set("slug", "new-slug")
	f.Set("title", "New Title")
	f.Set("body", "new body")
	f.Set("tags", "updated")
	f.Set("status", "published")
	f.Set("visibility", "public")

	resp := postArticle(app, "/admin/"+strconv.FormatInt(a.ID, 10), f)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	got, err := repo.GetArticle(context.Background(), a.ID)
	require.NoError(t, err)
	require.Equal(t, "new-slug", got.Slug)
	require.Equal(t, "New Title", got.Title)
	require.Equal(t, model.StatusPublished, got.Status)
	require.Equal(t, []string{"updated"}, got.Tags)
}

func TestArticleAdmin_Delete(t *testing.T) {
	app, repo := articleApp(t)
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "delete-me", Title: "Bye", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)

	resp := postArticle(app, "/admin/"+strconv.FormatInt(a.ID, 10)+"/delete", url.Values{})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/admin", resp.Header.Get("Location"))

	_, err = repo.GetArticle(context.Background(), a.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}
