package mcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/mcp"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func newTools(t *testing.T) (*mcp.Tools, store.Repository) {
	t.Helper()
	repo := inmem.New()
	clk := fixedClock{t: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)}
	return mcp.NewTools(repo, clk), repo
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func TestTools_ListArticles_Filters(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	_, _ = repo.CreateArticle(ctx, model.Article{
		Slug: "pub", Title: "Published Go", Body: "x", Type: model.ArticleTypeMarkdown,
		Status: model.StatusPublished, Visibility: model.VisibilityPublic,
	})
	_, _ = repo.CreateArticle(ctx, model.Article{
		Slug: "draft", Title: "Draft Rust", Body: "y", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})

	all, err := tools.ListArticles(ctx, mcp.ListInput{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	drafts, err := tools.ListArticles(ctx, mcp.ListInput{Status: "draft"})
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	require.Equal(t, "draft", drafts[0].Slug)

	found, err := tools.ListArticles(ctx, mcp.ListInput{Q: "Go"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "pub", found[0].Slug)
}

func TestTools_GetArticle_ByIDAndSlug(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, model.Article{
		Slug: "hello", Title: "Hello", Body: "body", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	require.NoError(t, err)

	byID, err := tools.GetArticle(ctx, mcp.GetInput{ID: a.ID})
	require.NoError(t, err)
	require.Equal(t, "Hello", byID.Title)

	bySlug, err := tools.GetArticle(ctx, mcp.GetInput{Slug: "hello"})
	require.NoError(t, err)
	require.Equal(t, a.ID, bySlug.ID)

	_, err = tools.GetArticle(ctx, mcp.GetInput{Slug: "missing"})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = tools.GetArticle(ctx, mcp.GetInput{})
	require.ErrorIs(t, err, mcp.ErrInvalidInput)
}

func TestTools_CreateArticle_DefaultsDraftPrivate(t *testing.T) {
	tools, _ := newTools(t)
	ctx := context.Background()
	a, err := tools.CreateArticle(ctx, mcp.CreateInput{
		Slug:  "new-post",
		Title: "New Post",
		Body:  "# Hi",
	})
	require.NoError(t, err)
	require.Equal(t, string(model.StatusDraft), a.Status)
	require.Equal(t, string(model.VisibilityPrivate), a.Visibility)
	require.Equal(t, string(model.ArticleTypeMarkdown), a.Type)
}

func TestTools_CreateArticle_WithSEOAndTags(t *testing.T) {
	tools, _ := newTools(t)
	ctx := context.Background()
	a, err := tools.CreateArticle(ctx, mcp.CreateInput{
		Slug:            "seo",
		Title:           "T",
		Body:            "b",
		Tags:            []string{"go", "web"},
		SEOTitle:        "SEO",
		Summary:         "sum",
		MetaDescription: "meta",
		CoverImageURL:   "/media/c.png",
		OGImageURL:      "/media/o.png",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"go", "web"}, a.Tags)
	require.Equal(t, "SEO", a.SEOTitle)
	require.Equal(t, "meta", a.MetaDescription)
}

func TestTools_CreateArticle_EmptySlug(t *testing.T) {
	tools, _ := newTools(t)
	_, err := tools.CreateArticle(context.Background(), mcp.CreateInput{Title: "T", Body: "b"})
	require.ErrorIs(t, err, store.ErrEmptySlug)
}

func TestTools_UpdateArticle_PatchFields(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, model.Article{
		Slug: "old", Title: "Old", Body: "old body", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate, Tags: []string{"a"},
	})
	require.NoError(t, err)

	title := "New Title"
	body := "new body"
	meta := "new meta"
	tags := []string{"b", "c"}
	updated, err := tools.UpdateArticle(ctx, mcp.UpdateInput{
		ID:              a.ID,
		Title:           &title,
		Body:            &body,
		MetaDescription: &meta,
		Tags:            &tags,
	})
	require.NoError(t, err)
	require.Equal(t, "New Title", updated.Title)
	require.Equal(t, "new body", updated.Body)
	require.Equal(t, "new meta", updated.MetaDescription)
	require.Equal(t, []string{"b", "c"}, updated.Tags)
	require.Equal(t, "old", updated.Slug, "slug unchanged when not patched")
}

func TestTools_SetStatus_PublishStampsPublishedAt(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	a, err := repo.CreateArticle(ctx, model.Article{
		Slug: "p", Title: "P", Body: "b", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	require.NoError(t, err)

	got, err := tools.SetArticleStatus(ctx, mcp.SetStatusInput{ID: a.ID, Status: "published"})
	require.NoError(t, err)
	require.Equal(t, string(model.StatusPublished), got.Status)
	require.NotNil(t, got.PublishedAt)
	require.Equal(t, time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC), got.PublishedAt.UTC())

	// Draft again clears published_at
	got, err = tools.SetArticleStatus(ctx, mcp.SetStatusInput{ID: a.ID, Status: "draft"})
	require.NoError(t, err)
	require.Equal(t, string(model.StatusDraft), got.Status)
	require.Nil(t, got.PublishedAt)
}

func TestTools_SetStatus_Invalid(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	a, _ := repo.CreateArticle(ctx, model.Article{
		Slug: "x", Title: "X", Body: "b", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	_, err := tools.SetArticleStatus(ctx, mcp.SetStatusInput{ID: a.ID, Status: "nope"})
	require.ErrorIs(t, err, mcp.ErrInvalidInput)
}

func TestTools_SetVisibility(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	a, _ := repo.CreateArticle(ctx, model.Article{
		Slug: "v", Title: "V", Body: "b", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	got, err := tools.SetArticleVisibility(ctx, mcp.SetVisibilityInput{ID: a.ID, Visibility: "public"})
	require.NoError(t, err)
	require.Equal(t, string(model.VisibilityPublic), got.Visibility)

	_, err = tools.SetArticleVisibility(ctx, mcp.SetVisibilityInput{ID: a.ID, Visibility: "secret"})
	require.ErrorIs(t, err, mcp.ErrInvalidInput)
}

func TestTools_PublicView_OmitsPassword(t *testing.T) {
	tools, repo := newTools(t)
	ctx := context.Background()
	a, _ := repo.CreateArticle(ctx, model.Article{
		Slug: "pw", Title: "Pw", Body: "b", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityProtected,
		Password: "H:secret",
	})
	view := tools.ToView(a)
	require.Equal(t, a.ID, view.ID)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "password")
	require.NotContains(t, string(raw), "H:secret")
}

func TestRequireToken(t *testing.T) {
	require.ErrorIs(t, mcp.ValidateToken(""), mcp.ErrUnauthorized)
	require.ErrorIs(t, mcp.ValidateToken("   "), mcp.ErrUnauthorized)
	require.NoError(t, mcp.ValidateToken("good-token"))
}
