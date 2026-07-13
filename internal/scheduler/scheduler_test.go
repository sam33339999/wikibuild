package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/scheduler"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func TestPublisher_Tick_PublishesDueDrafts(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(2 * time.Hour)

	due, err := repo.CreateArticle(ctx, model.Article{
		Slug: "due", Title: "Due", Body: "x", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
		PublishAt: &past,
	})
	require.NoError(t, err)
	later, err := repo.CreateArticle(ctx, model.Article{
		Slug: "later", Title: "Later", Body: "y", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
		PublishAt: &future,
	})
	require.NoError(t, err)

	p := &scheduler.Publisher{Repo: repo, Clock: clock.NewFake(now)}
	n, err := p.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	got, err := repo.GetArticle(ctx, due.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPublished, got.Status)
	require.NotNil(t, got.PublishedAt)
	require.Nil(t, got.PublishAt)
	require.True(t, got.PublishedAt.Equal(past.UTC()))

	still, err := repo.GetArticle(ctx, later.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusDraft, still.Status)
	require.NotNil(t, still.PublishAt)
}

func TestPublisher_Tick_IdempotentWhenNothingDue(t *testing.T) {
	repo := inmem.New()
	p := &scheduler.Publisher{Repo: repo, Clock: clock.NewFake(time.Now())}
	n, err := p.Tick(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
