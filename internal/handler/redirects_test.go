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

func TestRedirects_AdminCRUD(t *testing.T) {
	repo := inmem.New()
	h := handler.NewRedirects(repo)
	app := fiber.New()
	app.Get("/admin/redirects", h.List)
	app.Post("/admin/redirects", h.Create)
	app.Post("/admin/redirects/delete", h.Delete)

	form := url.Values{"from_path": {"old-slug"}, "to_path": {"/new-slug"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/redirects", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	got, err := repo.GetRedirect(context.Background(), "/old-slug")
	require.NoError(t, err)
	require.Equal(t, "/new-slug", got.ToPath)

	list, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/redirects", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(list.Body)
	require.Contains(t, string(body), "/old-slug")
}

func TestPublic_Article_FollowsRedirect(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "new-slug", "New Title", "body", model.StatusPublished, model.VisibilityPublic)
	_, err := repo.CreateRedirect(context.Background(), model.Redirect{
		FromPath: "/old-slug", ToPath: "/new-slug",
	})
	require.NoError(t, err)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/old-slug", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	require.Equal(t, "/new-slug", resp.Header.Get("Location"))
}

func TestArticleAdmin_SlugChangeCreatesRedirect(t *testing.T) {
	app, repo := articleApp(t)
	form := articleForm()
	form.Set("status", "published")
	resp := postArticle(app, "/admin/new", form)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	items, _, err := repo.ListArticles(context.Background(), store.ListQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	id := items[0].ID

	form.Set("slug", "renamed")
	form.Set("title", "Hello World")
	form.Set("body", "x")
	form.Set("status", "published")
	resp = postArticle(app, "/admin/"+itoa(int(id)), form)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	r, err := repo.GetRedirect(context.Background(), "/hello-world")
	require.NoError(t, err)
	require.Equal(t, "/renamed", r.ToPath)
}
