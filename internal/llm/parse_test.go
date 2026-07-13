package llm_test

import (
	"testing"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestParseSEOResult_CleanJSON(t *testing.T) {
	raw := `{
  "outline": ["Intro", "Details"],
  "meta_description": "A crisp meta under 155 chars.",
  "summary": "Two or three sentences for humans and feeds."
}`
	got, err := llm.ParseSEOResult(raw)
	require.NoError(t, err)
	require.Equal(t, []string{"Intro", "Details"}, got.Outline)
	require.Equal(t, "A crisp meta under 155 chars.", got.MetaDescription)
	require.Equal(t, "Two or three sentences for humans and feeds.", got.Summary)
}

func TestParseSEOResult_FencedJSON(t *testing.T) {
	raw := "Here you go:\n```json\n{\"outline\":[\"A\"],\"meta_description\":\"m\",\"summary\":\"s\"}\n```\n"
	got, err := llm.ParseSEOResult(raw)
	require.NoError(t, err)
	require.Equal(t, []string{"A"}, got.Outline)
	require.Equal(t, "m", got.MetaDescription)
	require.Equal(t, "s", got.Summary)
}

func TestParseSEOResult_EmbeddedObject(t *testing.T) {
	raw := `Sure! {"outline":["x"],"meta_description":"md","summary":"sum"} done.`
	got, err := llm.ParseSEOResult(raw)
	require.NoError(t, err)
	require.Equal(t, "md", got.MetaDescription)
	require.Equal(t, "sum", got.Summary)
}

func TestParseSEOResult_MissingFieldsError(t *testing.T) {
	_, err := llm.ParseSEOResult(`{"outline":[]}`)
	require.Error(t, err)
	require.ErrorIs(t, err, llm.ErrBadResponse)
}

func TestParseSEOResult_InvalidJSON(t *testing.T) {
	_, err := llm.ParseSEOResult(`not json at all`)
	require.Error(t, err)
	require.ErrorIs(t, err, llm.ErrBadResponse)
}

func TestParseSEOResult_TrimsWhitespace(t *testing.T) {
	raw := `{"outline":["  a  "],"meta_description":"  meta  ","summary":"  sum  "}`
	got, err := llm.ParseSEOResult(raw)
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, got.Outline)
	require.Equal(t, "meta", got.MetaDescription)
	require.Equal(t, "sum", got.Summary)
}
