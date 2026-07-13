//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/stretchr/testify/require"
)

func sampleArticle(slug string) model.Article {
	return model.Article{
		Slug:       slug,
		Title:      "Title " + slug,
		Type:       model.ArticleTypeMarkdown,
		Status:     model.StatusDraft,
		Visibility: model.VisibilityPublic,
		Body:       "body text",
		Tags:       []string{"go", "web"},
	}
}

func TestCreateArticle_AndGet(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()

	created, err := repo.CreateArticle(ctx, sampleArticle("hello"))
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.False(t, created.CreatedAt.IsZero(), "created_at should default to now()")
	require.False(t, created.UpdatedAt.IsZero())

	got, err := repo.GetArticle(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Slug, got.Slug)
	require.Equal(t, []string{"go", "web"}, got.Tags)
}

func TestCreateArticle_DuplicateSlug(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()

	_, err := repo.CreateArticle(ctx, sampleArticle("dup"))
	require.NoError(t, err)

	_, err = repo.CreateArticle(ctx, sampleArticle("dup"))
	require.ErrorIs(t, err, store.ErrDuplicateSlug)
}

func TestCreateArticle_EmptySlug(t *testing.T) {
	repo := NewTestRepo(t)
	a := sampleArticle("")
	_, err := repo.CreateArticle(context.Background(), a)
	require.ErrorIs(t, err, store.ErrEmptySlug)
}

func TestGetArticle_NotFound(t *testing.T) {
	repo := NewTestRepo(t)
	_, err := repo.GetArticle(context.Background(), 99999)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestGetArticleBySlug(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, sampleArticle("by-slug"))

	got, err := repo.GetArticleBySlug(ctx, "by-slug")
	require.NoError(t, err)
	require.Equal(t, "by-slug", got.Slug)
}

func TestUpdateArticle_SlugChange(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, sampleArticle("old"))
	require.NoError(t, err)

	a.Slug = "new"
	a.Title = "renamed"
	updated, err := repo.UpdateArticle(ctx, a)
	require.NoError(t, err)
	require.Equal(t, "new", updated.Slug)
	require.Equal(t, "renamed", updated.Title)

	_, err = repo.GetArticleBySlug(ctx, "old")
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.GetArticleBySlug(ctx, "new")
	require.NoError(t, err)
	require.Equal(t, a.ID, got.ID)
}

func TestUpdateArticle_PublishedAtRoundTrip(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, sampleArticle("pub"))
	require.NoError(t, err)

	want := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	a.PublishedAt = &want
	a.Status = model.StatusPublished
	updated, err := repo.UpdateArticle(ctx, a)
	require.NoError(t, err)
	require.NotNil(t, updated.PublishedAt)
	require.WithinDuration(t, want, *updated.PublishedAt, time.Second)
}

func TestDeleteArticle(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, sampleArticle("del"))
	require.NoError(t, err)

	require.NoError(t, repo.DeleteArticle(ctx, a.ID))
	_, err = repo.GetArticle(ctx, a.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestListArticles_FiltersAndPagination(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("a", model.StatusPublished, model.VisibilityPublic, "alpha"))
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("b", model.StatusDraft, model.VisibilityPublic, "beta"))
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("c", model.StatusPublished, model.VisibilityPrivate, "gamma"))

	items, total, err := repo.ListArticles(ctx, store.ListQuery{Status: model.StatusPublished})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, items, 2)

	// Limit + offset pagination.
	page, _, err := repo.ListArticles(ctx, store.ListQuery{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Len(t, page, 1)
}

func TestListArticles_TagFilter(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	tagged := sampleArticle("tagged")
	tagged.Tags = []string{"rare"}
	_, _ = repo.CreateArticle(ctx, tagged)
	_, _ = repo.CreateArticle(ctx, sampleArticle("untagged"))

	items, total, err := repo.ListArticles(ctx, store.ListQuery{Tag: "rare"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "tagged", items[0].Slug)
}

func TestListArticles_Search(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("s1", model.StatusDraft, model.VisibilityPublic, "Golang concurrency"))
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("s2", model.StatusDraft, model.VisibilityPublic, "unrelated"))

	items, total, err := repo.ListArticles(ctx, store.ListQuery{Search: "golang"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "s1", items[0].Slug)
}

func sampleArticleWith(slug string, status model.Status, vis model.Visibility, body string) model.Article {
	a := sampleArticle(slug)
	a.Status = status
	a.Visibility = vis
	a.Body = body
	return a
}

func TestCreateUser_AndGetByUsername(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, model.User{Username: "admin", PasswordHash: "$2a$10$hash"})
	require.NoError(t, err)
	require.NotZero(t, u.ID)
	require.False(t, u.CreatedAt.IsZero())

	got, err := repo.GetUserByUsername(ctx, "admin")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)
	require.Equal(t, "$2a$10$hash", got.PasswordHash)
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	_, err := repo.CreateUser(ctx, model.User{Username: "admin", PasswordHash: "h"})
	require.NoError(t, err)
	_, err = repo.CreateUser(ctx, model.User{Username: "admin", PasswordHash: "h2"})
	require.ErrorIs(t, err, store.ErrDuplicateUsername)
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	repo := NewTestRepo(t)
	_, err := repo.GetUserByUsername(context.Background(), "ghost")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestCreateUser_EmptyUsername(t *testing.T) {
	repo := NewTestRepo(t)
	_, err := repo.CreateUser(context.Background(), model.User{Username: "", PasswordHash: "h"})
	require.ErrorIs(t, err, store.ErrEmptyUsername)
}

func TestRepository_SatisfiesInterface(t *testing.T) {
	var _ store.Repository = (*Repository)(nil)
}
