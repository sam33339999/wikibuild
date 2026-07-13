package llm_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestClipBody_Empty(t *testing.T) {
	require.Empty(t, llm.ClipBody(""))
	require.Empty(t, llm.ClipBody("   \n\t  "))
}

func TestClipBody_UnderLimitUnchanged(t *testing.T) {
	body := "# Hello\n\nShort body."
	require.Equal(t, body, llm.ClipBody(body))
}

func TestClipBody_TruncatesOverMaxBytes(t *testing.T) {
	// Build a body larger than MaxBodyBytes using multi-byte runes so we
	// assert byte cap without leaving invalid UTF-8.
	unit := "測" // 3 bytes
	n := (llm.MaxBodyBytes / len(unit)) + 50
	body := strings.Repeat(unit, n)
	require.Greater(t, len(body), llm.MaxBodyBytes)

	got := llm.ClipBody(body)
	require.LessOrEqual(t, len(got), llm.MaxBodyBytes)
	require.True(t, utf8.ValidString(got), "truncated body must stay valid UTF-8")
	require.NotEmpty(t, got)
}

func TestBuildSEOMessages_ContainsTitleAndBody(t *testing.T) {
	msgs := llm.BuildSEOMessages("My Title", "# Body\n\nContent here.")
	require.Len(t, msgs, 2)
	require.Equal(t, "system", msgs[0].Role)
	require.Equal(t, "user", msgs[1].Role)

	sys := msgs[0].Content
	require.Contains(t, sys, "meta_description")
	require.Contains(t, sys, "summary")
	require.Contains(t, sys, "outline")
	require.Contains(t, sys, "JSON")

	user := msgs[1].Content
	require.Contains(t, user, "My Title")
	require.Contains(t, user, "Content here")
}

func TestBuildSEOMessages_ClipsHugeBody(t *testing.T) {
	body := strings.Repeat("x", llm.MaxBodyBytes+1000)
	msgs := llm.BuildSEOMessages("T", body)
	require.LessOrEqual(t, len(msgs[1].Content), llm.MaxBodyBytes+200) // title overhead
	require.NotContains(t, msgs[1].Content, strings.Repeat("x", llm.MaxBodyBytes+1))
}
