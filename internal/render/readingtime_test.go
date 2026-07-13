package render_test

import (
	"strings"
	"testing"

	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/stretchr/testify/require"
)

func TestReadingTime_EmptyIsZero(t *testing.T) {
	require.Equal(t, 0, render.ReadingTime(""))
	require.Equal(t, 0, render.ReadingTime("   \n\n  "))
}

func TestReadingTime_ShortEnglishOneMinute(t *testing.T) {
	// A few words → at least 1 minute.
	require.Equal(t, 1, render.ReadingTime("hello world"))
}

func TestReadingTime_EnglishByWordCount(t *testing.T) {
	// 400 words at ~200 wpm → 2 minutes.
	body := strings.Repeat("word ", 400)
	require.Equal(t, 2, render.ReadingTime(body))

	// 600 words → 3 minutes.
	body = strings.Repeat("word ", 600)
	require.Equal(t, 3, render.ReadingTime(body))
}

func TestReadingTime_ChineseByCharCount(t *testing.T) {
	// 600 Han chars at ~500 cpm → 2 minutes.
	body := strings.Repeat("中", 600)
	require.Equal(t, 2, render.ReadingTime(body))

	// 1200 Han chars → 3 minutes (ceil(1200/500)=3 → but 1200/500=2.4 → ceil 3).
	body = strings.Repeat("中", 1200)
	require.Equal(t, 3, render.ReadingTime(body))
}

func TestReadingTime_MixedContent(t *testing.T) {
	// 250 Han chars (0.5 min) + 100 English words (0.5 min) → ceil(1.0) = 1.
	body := strings.Repeat("中", 250) + " " + strings.Repeat("word ", 100)
	require.Equal(t, 1, render.ReadingTime(body))
}

func TestReadingTime_IgnoresMarkdownSyntax(t *testing.T) {
	// Markdown punctuation/markers shouldn't inflate the count much; just
	// ensure it returns a sensible positive number for real content.
	body := "# Title\n\nThis is a **bold** paragraph with `code`.\n\n## Section\n\nMore text here."
	require.GreaterOrEqual(t, render.ReadingTime(body), 1)
}
