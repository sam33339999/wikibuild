package inmem_test

import (
	"context"
	"testing"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func seedTagged(t *testing.T, repo store.Repository, slug string, tags ...string) model.Article {
	t.Helper()
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: slug, Title: slug, Type: model.ArticleTypeMarkdown,
		Status: model.StatusPublished, Visibility: model.VisibilityPublic,
		Tags: tags,
	})
	require.NoError(t, err)
	return a
}

func TestListTags_AggregatesAndSorts(t *testing.T) {
	repo := inmem.New()
	seedTagged(t, repo, "a", "go", "wiki")
	seedTagged(t, repo, "b", "go")
	seedTagged(t, repo, "c", "rust")
	seedTagged(t, repo, "d") // no tags

	tags, err := repo.ListTags(context.Background())
	require.NoError(t, err)
	require.Equal(t, []store.TagCount{
		{Name: "go", Count: 2},
		{Name: "rust", Count: 1},
		{Name: "wiki", Count: 1},
	}, tags)
}

func TestListTags_Empty(t *testing.T) {
	repo := inmem.New()
	tags, err := repo.ListTags(context.Background())
	require.NoError(t, err)
	require.Empty(t, tags)
}

func TestRenameTag_UpdatesAllArticles(t *testing.T) {
	repo := inmem.New()
	seedTagged(t, repo, "a", "old", "keep")
	seedTagged(t, repo, "b", "old")
	seedTagged(t, repo, "c", "other")

	n, err := repo.RenameTag(context.Background(), "old", "new")
	require.NoError(t, err)
	require.Equal(t, 2, n)

	a, _ := repo.GetArticleBySlug(context.Background(), "a")
	require.ElementsMatch(t, []string{"new", "keep"}, a.Tags)
	b, _ := repo.GetArticleBySlug(context.Background(), "b")
	require.Equal(t, []string{"new"}, b.Tags)
	c, _ := repo.GetArticleBySlug(context.Background(), "c")
	require.Equal(t, []string{"other"}, c.Tags)
}

func TestRenameTag_MergeDedupes(t *testing.T) {
	// Article already has both "from" and "to": merge must leave a single "to".
	repo := inmem.New()
	seedTagged(t, repo, "a", "from", "to", "x")

	n, err := repo.RenameTag(context.Background(), "from", "to")
	require.NoError(t, err)
	require.Equal(t, 1, n)

	a, _ := repo.GetArticleBySlug(context.Background(), "a")
	require.ElementsMatch(t, []string{"to", "x"}, a.Tags)
}

func TestRenameTag_EmptyNames(t *testing.T) {
	repo := inmem.New()
	_, err := repo.RenameTag(context.Background(), "", "x")
	require.ErrorIs(t, err, store.ErrEmptyTag)
	_, err = repo.RenameTag(context.Background(), "x", "")
	require.ErrorIs(t, err, store.ErrEmptyTag)
}

func TestRenameTag_SameNameIsNoop(t *testing.T) {
	repo := inmem.New()
	seedTagged(t, repo, "a", "go")
	n, err := repo.RenameTag(context.Background(), "go", "go")
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestRenameTag_UnknownFromAffectsZero(t *testing.T) {
	repo := inmem.New()
	seedTagged(t, repo, "a", "go")
	n, err := repo.RenameTag(context.Background(), "missing", "other")
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
