package handler_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"archive/zip"
	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func uploadApp(t *testing.T) (*fiber.App, store.Repository, string) {
	t.Helper()
	repo := inmem.New()
	dir := t.TempDir()
	h := handler.NewUpload(repo, dir)
	app := fiber.New()
	app.Get("/admin/upload", h.Form)
	app.Post("/admin/upload", h.Submit)
	return app, repo, dir
}

// makeZip builds an in-memory zip from name→content entries.
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// uploadRequest builds a multipart request carrying the metadata fields and
// one file upload under the "file" field.
func uploadRequest(method, path, slug, title, status, visibility, rawMode, filename string, fileBytes []byte) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("slug", slug)
	_ = mw.WriteField("title", title)
	_ = mw.WriteField("status", status)
	_ = mw.WriteField("visibility", visibility)
	if rawMode != "" {
		_ = mw.WriteField("raw_mode", rawMode)
	}
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(fileBytes)
	_ = mw.Close()
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestUpload_Form(t *testing.T) {
	app, _, _ := uploadApp(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/upload", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "<form")
}

func TestUpload_Zip_CreatesArticleAndExtractsFiles(t *testing.T) {
	app, repo, dir := uploadApp(t)
	zipBytes := makeZip(t, map[string]string{
		"index.html":    "<h1>Uploaded</h1>",
		"css/style.css": "body{color:red}",
	})

	resp, err := app.Test(uploadRequest(http.MethodPost, "/admin/upload",
		"my-post", "My Post", "published", "public", "on", "site.zip", zipBytes))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/admin", resp.Header.Get("Location"))

	// Article created as an html_upload pointing at index.html.
	a, err := repo.GetArticleBySlug(context.Background(), "my-post")
	require.NoError(t, err)
	require.Equal(t, model.ArticleTypeHTMLUpload, a.Type)
	require.Equal(t, "index.html", a.Body)
	require.True(t, a.RawMode, "raw_mode=on should persist")

	// Files extracted under contentDir/<slug>/.
	require.FileExists(t, filepath.Join(dir, "my-post", "index.html"))
	require.FileExists(t, filepath.Join(dir, "my-post", "css", "style.css"))
}

func TestUpload_Zip_StripsSingleRootFolderAndMacJunk(t *testing.T) {
	// Typical macOS zip: Folder/index.html + __MACOSX junk + nested assets.
	app, repo, dir := uploadApp(t)
	zipBytes := makeZip(t, map[string]string{
		"signoff-deck/index.html":           "<h1>Deck</h1>",
		"signoff-deck/slides/01-title.html": "<p>slide</p>",
		"__MACOSX/signoff-deck/._index.html": "junk",
		"__MACOSX/._signoff-deck":            "junk",
	})

	resp, err := app.Test(uploadRequest(http.MethodPost, "/admin/upload",
		"deck", "Deck", "published", "public", "on", "deck.zip", zipBytes))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	a, err := repo.GetArticleBySlug(context.Background(), "deck")
	require.NoError(t, err)
	require.Equal(t, "index.html", a.Body, "root folder must be stripped so entry is at slug root")

	require.FileExists(t, filepath.Join(dir, "deck", "index.html"))
	require.FileExists(t, filepath.Join(dir, "deck", "slides", "01-title.html"))
	// macOS junk must not be extracted
	_, err = os.Stat(filepath.Join(dir, "deck", "__MACOSX"))
	require.True(t, os.IsNotExist(err))
}

func TestUpload_HtmlFile_SavesAsIndex(t *testing.T) {
	app, repo, dir := uploadApp(t)
	html := []byte("<h1>Plain HTML</h1>")

	resp, err := app.Test(uploadRequest(http.MethodPost, "/admin/upload",
		"plain", "Plain", "draft", "public", "", "page.html", html))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	a, err := repo.GetArticleBySlug(context.Background(), "plain")
	require.NoError(t, err)
	require.Equal(t, model.ArticleTypeHTMLUpload, a.Type)
	require.False(t, a.RawMode)
	require.FileExists(t, filepath.Join(dir, "plain", "index.html"))
}

func TestUpload_InvalidSlug(t *testing.T) {
	app, _, _ := uploadApp(t)
	for _, slug := range []string{"../etc", "a/b", "", "has space"} {
		resp, err := app.Test(uploadRequest(http.MethodPost, "/admin/upload",
			slug, "T", "draft", "public", "", "index.html", []byte("<p>x</p>")))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "slug %q", slug)
	}
}

func TestUpload_DuplicateSlug(t *testing.T) {
	app, repo, _ := uploadApp(t)
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "taken", Title: "T", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})

	resp, err := app.Test(uploadRequest(http.MethodPost, "/admin/upload",
		"taken", "T", "draft", "public", "", "index.html", []byte("<p>x</p>")))
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestUpload_NoFile(t *testing.T) {
	app, _, _ := uploadApp(t)
	// POST with fields but no file part.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("slug", "nofile")
	_ = mw.WriteField("title", "T")
	_ = mw.WriteField("status", "draft")
	_ = mw.WriteField("visibility", "public")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpload_ZipSlip_Rejected(t *testing.T) {
	app, _, dir := uploadApp(t)
	zipBytes := makeZip(t, map[string]string{
		"index.html":    "<p>ok</p>",
		"../escape.txt": "should not be written",
	})

	resp, err := app.Test(uploadRequest(http.MethodPost, "/admin/upload",
		"slip", "Slip", "draft", "public", "", "site.zip", zipBytes))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Nothing escaped the slug dir.
	_, err = os.Stat(filepath.Join(dir, "escape.txt"))
	require.True(t, os.IsNotExist(err), "zip-slip entry must not be written outside the slug dir")
}
