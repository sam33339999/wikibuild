package mcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/mcp"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func TestTools_Execute_ListAndGet(t *testing.T) {
	repo := inmem.New()
	clk := fixedClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	tools := mcp.NewTools(repo, clk)
	ctx := context.Background()
	_, err := repo.CreateArticle(ctx, model.Article{
		Slug: "a", Title: "Alpha", Body: "body-a", Type: model.ArticleTypeMarkdown,
		Status: model.StatusPublished, Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)

	raw, err := tools.Execute(ctx, "list_articles", `{"q":"Alpha"}`)
	require.NoError(t, err)
	require.Contains(t, raw, "Alpha")
	require.NotContains(t, raw, "body-a", "list omits body")

	raw, err = tools.Execute(ctx, "get_article", `{"slug":"a"}`)
	require.NoError(t, err)
	require.Contains(t, raw, "body-a")
}

func TestTools_Execute_CreateDefaultDraftPrivate(t *testing.T) {
	repo := inmem.New()
	tools := mcp.NewTools(repo, fixedClock{t: time.Now()})
	raw, err := tools.Execute(context.Background(), "create_article", `{"slug":"n","title":"New"}`)
	require.NoError(t, err)
	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	require.Equal(t, "draft", v["status"])
	require.Equal(t, "private", v["visibility"])
}

func TestTools_Execute_UnknownTool(t *testing.T) {
	tools := mcp.NewTools(inmem.New(), nil)
	_, err := tools.Execute(context.Background(), "nope", `{}`)
	require.ErrorIs(t, err, mcp.ErrInvalidInput)
}

func TestTools_Execute_UpdateAndStatus(t *testing.T) {
	repo := inmem.New()
	tools := mcp.NewTools(repo, fixedClock{t: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)})
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, model.Article{
		Slug: "x", Title: "X", Body: "b", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	require.NoError(t, err)

	raw, err := tools.Execute(ctx, "update_article", `{"id":1,"title":"Y"}`)
	require.NoError(t, err)
	require.Contains(t, raw, `"title": "Y"`)

	raw, err = tools.Execute(ctx, "set_article_status", `{"id":1,"status":"published"}`)
	require.NoError(t, err)
	require.Contains(t, raw, "published")
	got, _ := repo.GetArticle(ctx, a.ID)
	require.Equal(t, model.StatusPublished, got.Status)
}
