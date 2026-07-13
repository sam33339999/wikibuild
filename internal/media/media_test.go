package media_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sam33339999/wikibuild/internal/media"
	"github.com/stretchr/testify/require"
)

// Minimal valid image headers so DetectContentType / our sniffers work.
var (
	pngHeader  = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	jpegHeader = []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 0, 0, 0, 0, 0, 0}
	gifHeader  = []byte("GIF89a..............")
	webpHeader = []byte("RIFF\x00\x00\x00\x00WEBP............")
)

func TestDetectExt_AllowedTypes(t *testing.T) {
	cases := []struct {
		data []byte
		ext  string
	}{
		{pngHeader, ".png"},
		{jpegHeader, ".jpg"},
		{gifHeader, ".gif"},
		{webpHeader, ".webp"},
	}
	for _, tc := range cases {
		ext, err := media.DetectExt(tc.data)
		require.NoError(t, err)
		require.Equal(t, tc.ext, ext)
	}
}

func TestDetectExt_RejectsNonImage(t *testing.T) {
	_, err := media.DetectExt([]byte("not an image at all"))
	require.ErrorIs(t, err, media.ErrUnsupportedType)

	_, err = media.DetectExt([]byte("<html>hi</html>"))
	require.ErrorIs(t, err, media.ErrUnsupportedType)
}

func TestDetectExt_Empty(t *testing.T) {
	_, err := media.DetectExt(nil)
	require.ErrorIs(t, err, media.ErrEmpty)
	_, err = media.DetectExt([]byte{})
	require.ErrorIs(t, err, media.ErrEmpty)
}

func TestSave_WritesUniqueFileAndReturnsURL(t *testing.T) {
	dir := t.TempDir()
	// Pad past the header so size is realistic.
	data := append(bytes.Clone(pngHeader), bytes.Repeat([]byte{1}, 64)...)

	got, err := media.Save(dir, data)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got.URL, "/media/"))
	require.True(t, strings.HasSuffix(got.URL, ".png"))
	require.Equal(t, filepath.Base(got.URL), got.Name)
	require.FileExists(t, filepath.Join(dir, got.Name))

	// Second save must not collide.
	got2, err := media.Save(dir, data)
	require.NoError(t, err)
	require.NotEqual(t, got.Name, got2.Name)
	require.FileExists(t, filepath.Join(dir, got2.Name))
}

func TestSave_RejectsTooLarge(t *testing.T) {
	dir := t.TempDir()
	// Build a payload larger than the limit with a valid PNG header.
	big := make([]byte, media.MaxBytes+1)
	copy(big, pngHeader)
	_, err := media.Save(dir, big)
	require.ErrorIs(t, err, media.ErrTooLarge)
}

func TestSave_CreatesDirIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "media")
	data := append(bytes.Clone(pngHeader), 0, 1, 2, 3)
	got, err := media.Save(dir, data)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, got.Name))
}

func TestSafeName_RejectsTraversal(t *testing.T) {
	require.False(t, media.SafeName("../etc/passwd"))
	require.False(t, media.SafeName("a/b.png"))
	require.False(t, media.SafeName(""))
	require.False(t, media.SafeName(".."))
	require.True(t, media.SafeName("abc123.png"))
	require.True(t, media.SafeName("a1b2c3d4e5f6.jpg"))
}

func TestOpen_ReadsSavedFile(t *testing.T) {
	dir := t.TempDir()
	data := append(bytes.Clone(pngHeader), 9, 8, 7)
	got, err := media.Save(dir, data)
	require.NoError(t, err)

	r, err := media.Open(dir, got.Name)
	require.NoError(t, err)
	defer r.Close()
	body, err := os.ReadFile(filepath.Join(dir, got.Name))
	require.NoError(t, err)
	require.Equal(t, data, body)
}

func TestOpen_RejectsUnsafeName(t *testing.T) {
	dir := t.TempDir()
	_, err := media.Open(dir, "../secret")
	require.ErrorIs(t, err, media.ErrUnsafeName)
}
