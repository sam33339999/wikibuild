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

func TestCreateArticle_NilTags(t *testing.T) {
	// The tags column is NOT NULL; nil tags must be stored as '{}' not NULL.
	repo := NewTestRepo(t)
	a := sampleArticle("no-tags")
	a.Tags = nil
	created, err := repo.CreateArticle(context.Background(), a)
	require.NoError(t, err)
	require.Empty(t, created.Tags)

	got, err := repo.GetArticle(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Tags)
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

func TestUpdateArticle_PinnedRoundTrip(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, sampleArticle("pin"))
	require.NoError(t, err)
	require.False(t, a.Pinned, "pinned defaults to false")

	a.Pinned = true
	updated, err := repo.UpdateArticle(ctx, a)
	require.NoError(t, err)
	require.True(t, updated.Pinned)

	got, err := repo.GetArticle(ctx, a.ID)
	require.NoError(t, err)
	require.True(t, got.Pinned)
}

func TestListArticles_PinnedFirst(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("a", model.StatusPublished, model.VisibilityPublic, "x"))
	p := sampleArticleWith("b", model.StatusPublished, model.VisibilityPublic, "y")
	p.Pinned = true
	_, _ = repo.CreateArticle(ctx, p)
	_, _ = repo.CreateArticle(ctx, sampleArticleWith("c", model.StatusPublished, model.VisibilityPublic, "z"))

	items, _, err := repo.ListArticles(ctx, store.ListQuery{})
	require.NoError(t, err)
	require.True(t, items[0].Pinned, "pinned article sorts first")
	require.Equal(t, "b", items[0].Slug)
}

func TestListTags_Aggregates(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	a := sampleArticle("t1")
	a.Tags = []string{"go", "wiki"}
	_, _ = repo.CreateArticle(ctx, a)
	b := sampleArticle("t2")
	b.Tags = []string{"go"}
	_, _ = repo.CreateArticle(ctx, b)

	tags, err := repo.ListTags(ctx)
	require.NoError(t, err)
	require.Equal(t, []store.TagCount{
		{Name: "go", Count: 2},
		{Name: "wiki", Count: 1},
	}, tags)
}

func TestRenameTag_MergeDedupes(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	a := sampleArticle("m1")
	a.Tags = []string{"from", "to", "x"}
	_, _ = repo.CreateArticle(ctx, a)
	b := sampleArticle("m2")
	b.Tags = []string{"from"}
	_, _ = repo.CreateArticle(ctx, b)

	n, err := repo.RenameTag(ctx, "from", "to")
	require.NoError(t, err)
	require.Equal(t, 2, n)

	got, err := repo.GetArticleBySlug(ctx, "m1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"to", "x"}, got.Tags)
	got2, err := repo.GetArticleBySlug(ctx, "m2")
	require.NoError(t, err)
	require.Equal(t, []string{"to"}, got2.Tags)
}

func TestPreviewTokenAndScheduleRoundTrip(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	past := time.Now().UTC().Add(-time.Hour)
	a := sampleArticle("sched")
	a.Status = model.StatusDraft
	a.PublishAt = &past
	a.PreviewToken = "tok123abc"
	created, err := repo.CreateArticle(ctx, a)
	require.NoError(t, err)
	require.Equal(t, "tok123abc", created.PreviewToken)
	require.NotNil(t, created.PublishAt)

	got, err := repo.GetArticleByPreviewToken(ctx, "tok123abc")
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	due, err := repo.ListDueScheduled(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, due)
}

func TestRedirects_Postgres(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()
	_, err := repo.CreateRedirect(ctx, model.Redirect{FromPath: "/a", ToPath: "/b"})
	require.NoError(t, err)
	got, err := repo.GetRedirect(ctx, "/a")
	require.NoError(t, err)
	require.Equal(t, "/b", got.ToPath)
	require.NoError(t, repo.DeleteRedirect(ctx, "/a"))
	_, err = repo.GetRedirect(ctx, "/a")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestSettings_GetSetRoundTrip(t *testing.T) {
	repo := NewTestRepo(t)
	ctx := context.Background()

	// Unset key returns "".
	got, err := repo.GetSetting(ctx, "default_protected_password")
	require.NoError(t, err)
	require.Empty(t, got)

	// Set then get.
	require.NoError(t, repo.SetSetting(ctx, "default_protected_password", "sitedefault"))
	got, err = repo.GetSetting(ctx, "default_protected_password")
	require.NoError(t, err)
	require.Equal(t, "sitedefault", got)

	// Upsert updates an existing key.
	require.NoError(t, repo.SetSetting(ctx, "default_protected_password", "newdefault"))
	got, err = repo.GetSetting(ctx, "default_protected_password")
	require.NoError(t, err)
	require.Equal(t, "newdefault", got)
}
