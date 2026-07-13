package handler_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func publicApp(t *testing.T) (*fiber.App, *inmem.Store, *auth.Signer, *clock.Fake, string) {
	t.Helper()
	repo := inmem.New()
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	signer := auth.NewSigner("supersecretkey1234", fc)
	dir := t.TempDir()
	h := handler.NewPublic(repo, signer, fakeHasher{}, "sitedefault", dir)
	app := fiber.New()
	app.Get("/", h.Index)
	app.Get("/:slug", h.Article)
	app.Get("/:slug/unlock", h.UnlockForm)
	app.Post("/:slug/unlock", h.UnlockSubmit)
	return app, repo, signer, fc, dir
}

func seedArticle(t *testing.T, repo *inmem.Store, slug, title, body string, status model.Status, vis model.Visibility) model.Article {
	t.Helper()
	pub := time.Unix(1_700_000_000, 0)
	a := model.Article{
		Slug: slug, Title: title, Body: body,
		Type: model.ArticleTypeMarkdown, Status: status, Visibility: vis,
	}
	if status == model.StatusPublished {
		a.PublishedAt = &pub
	}
	created, err := repo.CreateArticle(context.Background(), a)
	require.NoError(t, err)
	return created
}

func TestPublic_Index_ShowsOnlyPublishedPublic(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "pub", "Published", "x", model.StatusPublished, model.VisibilityPublic)
	seedArticle(t, repo, "draft", "Draft", "x", model.StatusDraft, model.VisibilityPublic)
	seedArticle(t, repo, "priv", "Private", "x", model.StatusPublished, model.VisibilityPrivate)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Published")
	require.NotContains(t, string(body), "Draft")
	require.NotContains(t, string(body), "Private")
}

func TestPublic_Index_Pagination(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	// Zero-padded titles avoid substring collisions ("Post 01" vs "Post 11").
	for i := 0; i < 12; i++ {
		seedArticle(t, repo, "p"+itoa(i), fmt.Sprintf("Post %02d", i), "x",
			model.StatusPublished, model.VisibilityPublic)
	}

	// Store returns newest-first, so page 1 = Post 11..Post 02.
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/?page=1", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Post 11")
	require.Contains(t, string(body), "Post 02")
	require.NotContains(t, string(body), "Post 01", "page 1 caps at page size")
	require.NotContains(t, string(body), "Post 00")

	// Page 2 = Post 01, Post 00.
	resp2, _ := app.Test(httptest.NewRequest(http.MethodGet, "/?page=2", nil))
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, _ := io.ReadAll(resp2.Body)
	require.Contains(t, string(body2), "Post 01")
	require.Contains(t, string(body2), "Post 00")
	require.NotContains(t, string(body2), "Post 11")

	// Page 3: empty (but still 200).
	resp3, _ := app.Test(httptest.NewRequest(http.MethodGet, "/?page=3", nil))
	require.Equal(t, http.StatusOK, resp3.StatusCode)
}

func TestPublic_Article_RendersMarkdown(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "hello", "Hello", "# Hello\n\nA **bold** word.", model.StatusPublished, model.VisibilityPublic)

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "<h1")
	require.Contains(t, string(body), "<strong>bold</strong>")
}

func TestPublic_Article_TOCRendered(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "toc", "TOC", "# Intro\n\n## Details\n\ntext", model.StatusPublished, model.VisibilityPublic)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/toc", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "intro")
	require.Contains(t, string(body), "details")
}

func TestPublic_Article_NotFound(t *testing.T) {
	app, _, _, _, _ := publicApp(t)
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/nope", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPublic_Article_DraftIs404(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "draft", "Draft", "x", model.StatusDraft, model.VisibilityPublic)
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/draft", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPublic_Article_NonPublicVisibilityIs404(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "priv", "Private", "x", model.StatusPublished, model.VisibilityPrivate)
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/priv", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- protected visibility + unlock flow ---

func seedProtected(t *testing.T, repo *inmem.Store, slug, title, password string) model.Article {
	t.Helper()
	pub := time.Unix(1_700_000_000, 0)
	a := model.Article{
		Slug: slug, Title: title, Body: "# Secret\n\nhidden text",
		Type: model.ArticleTypeMarkdown, Status: model.StatusPublished,
		Visibility: model.VisibilityProtected, Password: password, PublishedAt: &pub,
	}
	created, err := repo.CreateArticle(context.Background(), a)
	require.NoError(t, err)
	return created
}

func TestPublic_Protected_RedirectsToUnlock(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedProtected(t, repo, "secret", "Secret", "")

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/secret", nil))
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/secret/unlock", resp.Header.Get("Location"))
}

func TestPublic_UnlockForm_OK(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedProtected(t, repo, "secret", "Secret", "")

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/secret/unlock", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "受密碼保護")
	require.Contains(t, string(body), "<form")
}

func TestPublic_UnlockForm_NonProtectedIs404(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "pub", "Public", "x", model.StatusPublished, model.VisibilityPublic)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/pub/unlock", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPublic_UnlockSubmit_SiteDefault_CorrectSetsCookie(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedProtected(t, repo, "secret", "Secret", "") // no article password → site default "sitedefault"

	resp := postUnlock(app, "/secret/unlock", "sitedefault")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/secret", resp.Header.Get("Location"))
	require.Contains(t, resp.Header.Get("Set-Cookie"), "wikibuild_unlock_")
}

func TestPublic_UnlockSubmit_ArticlePassword_Correct(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	// fakeHasher hashes to "H:"+plain, so the stored hash is "H:mypass".
	seedProtected(t, repo, "secret", "Secret", "H:mypass")

	resp := postUnlock(app, "/secret/unlock", "mypass")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/secret", resp.Header.Get("Location"))
}

func TestPublic_UnlockSubmit_WrongPassword_RerendersWithError(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedProtected(t, repo, "secret", "Secret", "")

	resp := postUnlock(app, "/secret/unlock", "wrong")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "密碼不正確")
}

func TestPublic_AfterUnlock_ArticleRenders(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	a := seedProtected(t, repo, "secret", "Secret", "")

	// Unlock with the site default password, collecting the cookie.
	resp := postUnlock(app, "/secret/unlock", "sitedefault")
	var unlockCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "wikibuild_unlock_"+itoa(int(a.ID)) {
			unlockCookie = c
		}
	}
	require.NotNil(t, unlockCookie, "unlock cookie must be set")

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.AddCookie(unlockCookie)
	view, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, view.StatusCode)
	body, _ := io.ReadAll(view.Body)
	require.Contains(t, string(body), "hidden text")
}

func TestPublic_Protected_AdminBypassesUnlock(t *testing.T) {
	app, repo, signer, _, _ := publicApp(t)
	seedProtected(t, repo, "secret", "Secret", "")

	tok, err := signer.Sign("admin", time.Hour)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.AddCookie(&http.Cookie{Name: "wikibuild_admin", Value: tok})
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func postUnlock(app *fiber.App, path, password string) *http.Response {
	form := url.Values{}
	form.Set("password", password)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := app.Test(req)
	return resp
}

// --- html_upload serving (M3.2) ---

func seedHTMLUpload(t *testing.T, repo *inmem.Store, dir, slug, title, html string, rawMode bool) model.Article {
	t.Helper()
	pub := time.Unix(1_700_000_000, 0)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, slug), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, slug, "index.html"), []byte(html), 0o644))
	a := model.Article{
		Slug: slug, Title: title, Type: model.ArticleTypeHTMLUpload, Body: "index.html",
		Status: model.StatusPublished, Visibility: model.VisibilityPublic,
		RawMode: rawMode, PublishedAt: &pub,
	}
	created, err := repo.CreateArticle(context.Background(), a)
	require.NoError(t, err)
	return created
}

func TestPublic_HtmlUpload_RawMode_ServedAsIs(t *testing.T) {
	app, repo, _, _, dir := publicApp(t)
	fullDoc := "<!doctype html><html><head><title>Raw</title></head><body><p>raw content</p></body></html>"
	seedHTMLUpload(t, repo, dir, "raw", "Raw", fullDoc, true)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/raw", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	// Raw mode serves the file verbatim — no layout chrome added.
	require.Equal(t, fullDoc, string(body))
}

func TestPublic_HtmlUpload_InjectedIntoLayout(t *testing.T) {
	app, repo, _, _, dir := publicApp(t)
	content := "<p>injected content</p>"
	seedHTMLUpload(t, repo, dir, "inj", "Injected", content, false)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/inj", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	// Injected: wrapped in the layout (has the article title) AND the content.
	require.Contains(t, string(body), "Injected")
	require.Contains(t, string(body), "injected content")
}

// --- wikilinks + backlinks (M4.2) ---

func TestPublic_WikilinkRenderedAsLink(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	seedArticle(t, repo, "alpha", "Alpha", "Link to [[beta]] here.", model.StatusPublished, model.VisibilityPublic)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/alpha", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), `<a href="/beta">beta</a>`)
}

func TestPublic_BacklinksListed(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	// alpha links to beta via a wikilink.
	seedArticle(t, repo, "alpha", "Alpha Post", "See [[beta]] for more.", model.StatusPublished, model.VisibilityPublic)
	// beta is the link target.
	seedArticle(t, repo, "beta", "Beta Post", "Beta content.", model.StatusPublished, model.VisibilityPublic)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/beta", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "連回此頁的文章")
	require.Contains(t, string(body), `<a href="/alpha">Alpha Post</a>`)
}

func TestPublic_Backlinks_ExcludeSelfAndNonPublic(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	// beta self-links and a private article links to beta.
	seedArticle(t, repo, "beta", "Beta", "Self [[beta]] link.", model.StatusPublished, model.VisibilityPublic)
	seedArticle(t, repo, "secret", "Secret", "Links [[beta]].", model.StatusPublished, model.VisibilityPrivate)

	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/beta", nil))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.NotContains(t, string(body), "Secret", "private backlink must not show")
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return string(b)
}
