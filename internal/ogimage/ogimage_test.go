package ogimage_test

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/sam33339999/wikibuild/internal/ogimage"
	"github.com/stretchr/testify/require"
)

func TestRender_PNGDimensions(t *testing.T) {
	raw, err := ogimage.Render("Hello World", "WikiBuild")
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(raw, []byte{0x89, 'P', 'N', 'G'}), "PNG magic")

	img, err := png.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	b := img.Bounds()
	require.Equal(t, ogimage.Width, b.Dx())
	require.Equal(t, ogimage.Height, b.Dy())
}

func TestRender_LongTitleDoesNotPanic(t *testing.T) {
	title := "這是一篇非常非常長的中文標題用來測試自動換行與截斷行為是否穩定不 panic"
	raw, err := ogimage.Render(title, "My Site")
	require.NoError(t, err)
	require.Greater(t, len(raw), 1000)
}

func TestRender_EmptyTitleUsesFallback(t *testing.T) {
	raw, err := ogimage.Render("  ", "Site")
	require.NoError(t, err)
	_, err = png.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
}
