package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func editorSearchApp(t *testing.T) (*fiber.App, store.Repository) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewArticleAdmin(repo, fakeHasher{}, nil, t.TempDir(), false)
	app := fiber.New()
	app.Get("/admin/api/articles/search", h.SearchJSON)
	return app, repo
}

func TestArticleAdmin_SearchJSON_EmptyQuery(t *testing.T) {
	app, _ := editorSearchApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/api/articles/search", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var items []any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
	require.Empty(t, items)
}

func TestArticleAdmin_SearchJSON_FindsDraftAndPublished(t *testing.T) {
	app, repo := editorSearchApp(t)
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "go-tips", Title: "Go Tips", Body: "interfaces",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "rust-book", Title: "Rust Book", Body: "ownership",
		Type: model.ArticleTypeMarkdown, Status: model.StatusPublished, Visibility: model.VisibilityPublic,
	})
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "other", Title: "Unrelated", Body: "zzz",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/api/articles/search?q=go", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var items []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
	require.Len(t, items, 1)
	require.Equal(t, "go-tips", items[0]["slug"])
	require.Equal(t, "Go Tips", items[0]["title"])
	require.Equal(t, "draft", items[0]["status"])
	require.Equal(t, "private", items[0]["visibility"])
	// id present for exclude filtering
	require.NotZero(t, items[0]["id"])
}

func TestArticleAdmin_SearchJSON_ExcludeID(t *testing.T) {
	app, repo := editorSearchApp(t)
	a, _ := repo.CreateArticle(context.Background(), model.Article{
		Slug: "self", Title: "Self Link Topic", Body: "x",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "other-topic", Title: "Other Topic", Body: "y",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})

	url := "/admin/api/articles/search?q=topic&exclude_id=" + strconv.FormatInt(a.ID, 10)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, url, nil))
	require.NoError(t, err)
	var items []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
	require.Len(t, items, 1)
	require.Equal(t, "other-topic", items[0]["slug"])
}

func TestArticleAdmin_SearchJSON_RespectsLimit(t *testing.T) {
	app, repo := editorSearchApp(t)
	for i := 0; i < 25; i++ {
		_, _ = repo.CreateArticle(context.Background(), model.Article{
			Slug: "p" + strconv.Itoa(i), Title: "Match " + strconv.Itoa(i), Body: "match body",
			Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
		})
	}
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/api/articles/search?q=match", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	var items []map[string]any
	require.NoError(t, json.Unmarshal(body, &items))
	require.LessOrEqual(t, len(items), 20)
	require.Equal(t, 20, len(items))
}

func TestArticleAdmin_NewForm_HasEditorSearchPanel(t *testing.T) {
	app, _ := articleApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/new", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, `id="editor-search"`)
	require.Contains(t, s, "/static/js/editor-search.js")
	require.Contains(t, s, "站內文章")
}
