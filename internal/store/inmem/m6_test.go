package inmem_test

import (
	"context"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func TestGetArticleByPreviewToken(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	_, err := repo.CreateArticle(ctx, model.Article{
		Slug: "d", Title: "Draft", Status: model.StatusDraft,
		Type: model.ArticleTypeMarkdown, PreviewToken: "tok-abc",
	})
	require.NoError(t, err)

	got, err := repo.GetArticleByPreviewToken(ctx, "tok-abc")
	require.NoError(t, err)
	require.Equal(t, "d", got.Slug)

	_, err = repo.GetArticleByPreviewToken(ctx, "nope")
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.GetArticleByPreviewToken(ctx, "")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestListDueScheduled(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	due, _ := repo.CreateArticle(ctx, model.Article{
		Slug: "due", Title: "Due", Status: model.StatusDraft,
		Type: model.ArticleTypeMarkdown, PublishAt: &past,
	})
	_, _ = repo.CreateArticle(ctx, model.Article{
		Slug: "later", Title: "Later", Status: model.StatusDraft,
		Type: model.ArticleTypeMarkdown, PublishAt: &future,
	})
	_, _ = repo.CreateArticle(ctx, model.Article{
		Slug: "pub", Title: "Pub", Status: model.StatusPublished,
		Type: model.ArticleTypeMarkdown, PublishAt: &past,
	})
	_, _ = repo.CreateArticle(ctx, model.Article{
		Slug: "nosched", Title: "No", Status: model.StatusDraft,
		Type: model.ArticleTypeMarkdown,
	})

	items, err := repo.ListDueScheduled(ctx, now)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, due.ID, items[0].ID)
}

func TestRedirects_CRUD(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()

	r, err := repo.CreateRedirect(ctx, model.Redirect{FromPath: "/old", ToPath: "/new"})
	require.NoError(t, err)
	require.NotZero(t, r.ID)

	got, err := repo.GetRedirect(ctx, "/old")
	require.NoError(t, err)
	require.Equal(t, "/new", got.ToPath)

	// Upsert on conflict.
	_, err = repo.CreateRedirect(ctx, model.Redirect{FromPath: "/old", ToPath: "/newer"})
	require.NoError(t, err)
	got, _ = repo.GetRedirect(ctx, "/old")
	require.Equal(t, "/newer", got.ToPath)

	list, err := repo.ListRedirects(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, repo.DeleteRedirect(ctx, "/old"))
	_, err = repo.GetRedirect(ctx, "/old")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestRedirects_EmptyPath(t *testing.T) {
	repo := inmem.New()
	_, err := repo.CreateRedirect(context.Background(), model.Redirect{FromPath: "", ToPath: "/x"})
	require.ErrorIs(t, err, store.ErrEmptyPath)
}
