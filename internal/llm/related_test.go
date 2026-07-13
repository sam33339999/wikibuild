package llm_test

import (
	"testing"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestBuildRelatedMessages_IncludesSelectionAndCatalog(t *testing.T) {
	msgs := llm.BuildRelatedMessages("see wikilinks", []llm.CatalogEntry{
		{Slug: "go-tips", Title: "Go Tips", Tags: []string{"go"}, Summary: "about go"},
		{Slug: "rust", Title: "Rust", Tags: nil, Summary: ""},
	})
	require.Len(t, msgs, 2)
	require.Equal(t, "system", msgs[0].Role)
	require.Contains(t, msgs[0].Content, "suggestions")
	require.Contains(t, msgs[1].Content, "see wikilinks")
	require.Contains(t, msgs[1].Content, "go-tips")
	require.Contains(t, msgs[1].Content, "Go Tips")
	require.Contains(t, msgs[1].Content, "rust")
}

func TestParseRelatedResult_OK(t *testing.T) {
	raw := `{"suggestions":[{"slug":"go-tips","title":"Go Tips","reason":"same topic"},{"slug":"x","title":"X","reason":"y"}]}`
	got, err := llm.ParseRelatedResult(raw)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "go-tips", got[0].Slug)
	require.Equal(t, "same topic", got[0].Reason)
}

func TestParseRelatedResult_Fenced(t *testing.T) {
	raw := "```json\n{\"suggestions\":[{\"slug\":\"a\",\"title\":\"A\",\"reason\":\"r\"}]}\n```"
	got, err := llm.ParseRelatedResult(raw)
	require.NoError(t, err)
	require.Equal(t, "a", got[0].Slug)
}

func TestParseRelatedResult_EmptyOrInvalid(t *testing.T) {
	_, err := llm.ParseRelatedResult(`{}`)
	require.ErrorIs(t, err, llm.ErrBadResponse)
	_, err = llm.ParseRelatedResult(`not json`)
	require.ErrorIs(t, err, llm.ErrBadResponse)
}
