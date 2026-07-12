package inmem_test

import (
	"context"
	"testing"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func TestCreateArticle_DuplicateSlug(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	a := model.Article{
		Slug:       "hello",
		Title:      "Hi",
		Type:       model.ArticleTypeMarkdown,
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
	}
	_, err := repo.CreateArticle(ctx, a)
	require.NoError(t, err)

	_, err = repo.CreateArticle(ctx, a)
	require.ErrorIs(t, err, store.ErrDuplicateSlug)
}

func TestGetArticleBySlug_NotFound(t *testing.T) {
	repo := inmem.New()
	_, err := repo.GetArticleBySlug(context.Background(), "nope")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestUpdateArticle_SlugChangeRemovesOldMapping(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, model.Article{Slug: "old", Title: "T"})
	require.NoError(t, err)

	a.Slug = "new"
	_, err = repo.UpdateArticle(ctx, a)
	require.NoError(t, err)

	_, err = repo.GetArticleBySlug(ctx, "old")
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.GetArticleBySlug(ctx, "new")
	require.NoError(t, err)
	require.Equal(t, a.ID, got.ID)
}

func TestListArticles_FiltersByStatus(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, model.Article{Slug: "p", Status: model.StatusPublished, Visibility: model.VisibilityPublic})
	_, _ = repo.CreateArticle(ctx, model.Article{Slug: "d", Status: model.StatusDraft, Visibility: model.VisibilityPublic})

	items, total, err := repo.ListArticles(ctx, store.ListQuery{Status: model.StatusPublished})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "p", items[0].Slug)
}
