package seo_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/seo"
	"github.com/stretchr/testify/require"
)

func TestArticleJSONLD_EmptyBaseSkips(t *testing.T) {
	require.Empty(t, seo.ArticleJSONLD("", model.Article{Slug: "a", Title: "T"}, "d"))
}

func TestArticleJSONLD_ValidBlogPosting(t *testing.T) {
	pub := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	raw := seo.ArticleJSONLD("https://ex.com", model.Article{
		Slug: "hello", Title: "Hello World", PublishedAt: &pub,
	}, "A short desc")
	require.NotEmpty(t, raw)

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	require.Equal(t, "https://schema.org", m["@context"])
	require.Equal(t, "BlogPosting", m["@type"])
	require.Equal(t, "Hello World", m["headline"])
	require.Equal(t, "https://ex.com/hello", m["url"])
	require.Equal(t, "A short desc", m["description"])
	require.Contains(t, m["datePublished"], "2024-03-01")
}
