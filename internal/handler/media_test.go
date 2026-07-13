package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/media"
	"github.com/stretchr/testify/require"
)

var (
	testPNG  = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 1, 2, 3, 4, 5, 6, 7}
	testJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 0, 0, 1, 2, 3, 4}
)

func mediaApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	dir := t.TempDir()
	h := handler.NewMedia(dir)
	// Match production: BodyLimit above media.MaxBytes so the handler's own
	// size check runs (Fiber's default 4MiB is below MaxBytes).
	app := fiber.New(fiber.Config{BodyLimit: media.MaxBytes + 512*1024})
	app.Post("/admin/media", h.Upload)
	app.Get("/media/:name", h.Serve)
	return app, dir
}

func mediaMultipart(filename string, data []byte) (*http.Request, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/media", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req, mw.FormDataContentType()
}

func TestMedia_UploadPNG_ReturnsURLAndSavesFile(t *testing.T) {
	app, dir := mediaApp(t)
	req, _ := mediaMultipart("shot.png", testPNG)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body["url"])
	require.NotEmpty(t, body["name"])
	require.Contains(t, body["url"], "/media/")
	require.True(t, filepath.Ext(body["name"]) == ".png")
	require.FileExists(t, filepath.Join(dir, body["name"]))
}

func TestMedia_UploadJPEG(t *testing.T) {
	app, _ := mediaApp(t)
	req, _ := mediaMultipart("pic.jpg", testJPEG)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Contains(t, body["name"], ".jpg")
}

func TestMedia_UploadRejectsNonImage(t *testing.T) {
	app, _ := mediaApp(t)
	req, _ := mediaMultipart("x.txt", []byte("hello world not an image"))
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
}

func TestMedia_UploadRejectsMissingFile(t *testing.T) {
	app, _ := mediaApp(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/media", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMedia_UploadRejectsTooLarge(t *testing.T) {
	app, _ := mediaApp(t)
	big := make([]byte, media.MaxBytes+1)
	copy(big, testPNG)
	req, _ := mediaMultipart("huge.png", big)
	// Fiber may buffer the whole body; raise Test body limit via config if needed.
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestMedia_Serve_ReturnsSavedImage(t *testing.T) {
	app, dir := mediaApp(t)
	// Seed a file directly.
	name := "deadbeef.png"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), testPNG, 0o644))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/media/"+name, nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, testPNG, body)
}

func TestMedia_Serve_NotFound(t *testing.T) {
	app, _ := mediaApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/media/nope.png", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestMedia_Serve_RejectsTraversal(t *testing.T) {
	app, _ := mediaApp(t)
	// Path params with ".." should not escape; SafeName rejects them.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/media/..%2Fsecret", nil))
	require.NoError(t, err)
	// Either 404 (unsafe/not found) — never 200 with leaked content.
	require.NotEqual(t, http.StatusOK, resp.StatusCode)
}
