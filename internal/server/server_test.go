package server_test

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/server"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

// fakeHasher mirrors bcrypt's contract without cost, keeping the assembled-app
// test fast.
type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "H:" + p, nil }
func (fakeHasher) Compare(h, p string) error {
	if h == "H:"+p {
		return nil
	}
	return auth.ErrPasswordMismatch
}

func buildApp(t *testing.T) *fiber.App {
	t.Helper()
	repo := inmem.New()
	_, err := repo.CreateUser(t.Context(), model.User{Username: "admin", PasswordHash: "H:s3cret"})
	require.NoError(t, err)

	root := t.TempDir()
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	// Tests run with package dir as cwd (internal/server); theme assets live at repo root.
	staticDir := filepath.Join("..", "..", "static")
	app := server.New(server.Deps{
		Store:           repo,
		Hasher:          fakeHasher{},
		Signer:          auth.NewSigner("supersecretkey1234", fc),
		Limiter:         auth.NewLoginLimiter(fc, auth.DefaultLimiterConfig()),
		Clock:           fc,
		SiteDefaultPass: "sitedefault",
		ContentDir:      root + "/uploads",
		MediaDir:        root + "/media",
		StaticDir:       staticDir,
		BaseURL:         "https://ex.com",
		SiteTitle:       "Test Site",
	})
	return app
}

// csrfTokenAndCookie GETs the login page and returns the embedded token plus
// the response cookies, so a subsequent POST can replay them.
func csrfTokenAndCookie(t *testing.T, app *fiber.App) (string, []*http.Cookie) {
	t.Helper()
	resp, err := do(app, http.MethodGet, "/admin/login", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	m := regexp.MustCompile(`name="_csrf" value="([^"]+)"`).FindSubmatch(body)
	require.Len(t, m, 2, "login form must embed a csrf token")
	return string(m[1]), resp.Cookies()
}

// do issues a request. For urlencoded form bodies it sets the Content-Type.
// Multipart uploads must build the *http.Request manually with the proper
// multipart Content-Type (this helper would mislabel them) — see
// TestServer_UploadZip_ServesRawAndInjected.
func do(app *fiber.App, method, path string, body io.Reader, cookies []*http.Cookie) (*http.Response, error) {
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return app.Test(req)
}

func TestServer_LoginPageEmitsCSRFToken(t *testing.T) {
	app := buildApp(t)
	tok, _ := csrfTokenAndCookie(t, app)
	require.NotEmpty(t, tok)
}

func TestServer_PostLoginWithoutCSRF_IsRejected(t *testing.T) {
	app := buildApp(t)
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "s3cret")
	resp, err := do(app, http.MethodPost, "/admin/login",
		strings.NewReader(form.Encode()), nil)
	require.NoError(t, err)
	// Readable redirect instead of bare 403 Forbidden.
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "missing csrf redirects to login")
	require.Equal(t, "/admin/login?err=csrf", resp.Header.Get("Location"))
}

func TestServer_FullLoginFlow_WithCSRF(t *testing.T) {
	app := buildApp(t)
	tok, cookies := csrfTokenAndCookie(t, app)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "s3cret")
	form.Set("_csrf", tok)

	resp, err := do(app, http.MethodPost, "/admin/login",
		strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/admin", resp.Header.Get("Location"))

	// Collect the session cookie issued on login.
	for _, c := range resp.Cookies() {
		if c.Name == "wikibuild_admin" {
			cookies = append(cookies, c)
		}
	}

	// Authenticated dashboard access.
	dash, err := do(app, http.MethodGet, "/admin", nil, cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dash.StatusCode)
}

func TestServer_DashboardWithoutSession_RedirectsToLogin(t *testing.T) {
	app := buildApp(t)
	resp, err := do(app, http.MethodGet, "/admin", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/admin/login", resp.Header.Get("Location"))
}

// loginSession performs the CSRF login flow and returns cookies carrying
// both the CSRF and session cookies for authenticated requests.
func loginSession(t *testing.T, app *fiber.App) []*http.Cookie {
	t.Helper()
	tok, cookies := csrfTokenAndCookie(t, app)
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "s3cret")
	form.Set("_csrf", tok)
	resp, err := do(app, http.MethodPost, "/admin/login",
		strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	for _, c := range resp.Cookies() {
		if c.Name == "wikibuild_admin" {
			cookies = append(cookies, c)
		}
	}
	return cookies
}

// getCSRF refreshes the CSRF token for an authenticated session by reading
// the new-article form (the CSRF middleware issues a fresh token per request).
func getCSRF(t *testing.T, app *fiber.App, path string, cookies []*http.Cookie) string {
	t.Helper()
	resp, err := do(app, http.MethodGet, path, nil, cookies)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	m := regexp.MustCompile(`name="_csrf" value="([^"]+)"`).FindSubmatch(body)
	require.Len(t, m, 2, "form at %s must embed a csrf token", path)
	// The response may also set a refreshed csrf cookie; fold it in.
	for _, c := range resp.Cookies() {
		if c.Name == "csrf_" {
			cookies = append(cookies, c)
		}
	}
	return string(m[1])
}

func TestServer_ArticleCRUD_FullFlow(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)

	// Create an article via the authenticated, CSRF-protected form.
	tok := getCSRF(t, app, "/admin/new", cookies)
	form := url.Values{}
	form.Set("slug", "first-post")
	form.Set("title", "First Post")
	form.Set("body", "# Hello\n\nWorld.")
	form.Set("tags", "go, web")
	form.Set("status", "published")
	form.Set("visibility", "public")
	form.Set("_csrf", tok)
	resp, err := do(app, http.MethodPost, "/admin/new",
		strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	editURL := resp.Header.Get("Location")
	require.True(t, strings.HasPrefix(editURL, "/admin/") && strings.HasSuffix(editURL, "/edit"))

	// The list page shows the new article.
	list, err := do(app, http.MethodGet, "/admin", nil, cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, list.StatusCode)
	listBody, _ := io.ReadAll(list.Body)
	require.Contains(t, string(listBody), "First Post")

	// The edit form is pre-filled.
	edit, err := do(app, http.MethodGet, editURL, nil, cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, edit.StatusCode)
	editBody, _ := io.ReadAll(edit.Body)
	require.Contains(t, string(editBody), "First Post")
	require.Contains(t, string(editBody), "# Hello")
}

func TestServer_ArticleCRUD_RequiresAuth(t *testing.T) {
	app := buildApp(t)
	// Without a session, every admin article route redirects to login.
	for _, p := range []string{"/admin", "/admin/new", "/admin/1/edit"} {
		resp, err := do(app, http.MethodGet, p, nil, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusFound, resp.StatusCode, p)
		require.Equal(t, "/admin/login", resp.Header.Get("Location"))
	}
}

func TestServer_SettingsPage_UpdatesDefaultPassword(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)

	// GET the settings form.
	formResp, err := do(app, http.MethodGet, "/admin/settings", nil, cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, formResp.StatusCode)
	formBody, _ := io.ReadAll(formResp.Body)
	m := regexp.MustCompile(`name="_csrf" value="([^"]+)"`).FindSubmatch(formBody)
	require.Len(t, m, 2)

	// Save a new default protected password.
	form := url.Values{}
	form.Set("default_protected_password", "newsitedefault")
	form.Set("_csrf", string(m[1]))
	save, err := do(app, http.MethodPost, "/admin/settings",
		strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, save.StatusCode)
	require.Equal(t, "/admin/settings", save.Header.Get("Location"))

	// A protected article with no per-article password now unlocks with the
	// new site default. Create one, then unlock.
	tok := getCSRF(t, app, "/admin/new", cookies)
	artForm := url.Values{}
	artForm.Set("slug", "prot")
	artForm.Set("title", "Prot")
	artForm.Set("body", "secret body")
	artForm.Set("status", "published")
	artForm.Set("visibility", "protected")
	artForm.Set("_csrf", tok)
	do(app, http.MethodPost, "/admin/new", strings.NewReader(artForm.Encode()), cookies)

	// Unlock with the NEW site default.
	unlockForm, _ := do(app, http.MethodGet, "/prot/unlock", nil, nil)
	ub, _ := io.ReadAll(unlockForm.Body)
	um := regexp.MustCompile(`name="_csrf" value="([^"]+)"`).FindSubmatch(ub)
	postForm := url.Values{}
	postForm.Set("password", "newsitedefault")
	postForm.Set("_csrf", string(um[1]))
	submit, err := do(app, http.MethodPost, "/prot/unlock",
		strings.NewReader(postForm.Encode()), unlockForm.Cookies())
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, submit.StatusCode, "new site default must unlock")
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

// cookiesByName returns only the cookies whose Name matches.
func cookiesByName(cookies []*http.Cookie, names ...string) []*http.Cookie {
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[n] = struct{}{}
	}
	out := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		if _, ok := want[c.Name]; ok {
			out = append(out, c)
		}
	}
	return out
}

func TestServer_UploadZip_ServesRawAndInjected(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)

	// Send only the session cookie (not any stale csrf_ from login) so the
	// upload form GET issues a single fresh csrf token+cookie (mirrors a real
	// browser, which keeps one csrf_ cookie).
	sessionOnly := cookiesByName(cookies, "wikibuild_admin")
	formResp, err := do(app, http.MethodGet, "/admin/upload", nil, sessionOnly)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, formResp.StatusCode)
	fb, _ := io.ReadAll(formResp.Body)
	m := regexp.MustCompile(`name="_csrf" value="([^"]+)"`).FindSubmatch(fb)
	require.Len(t, m, 2)
	postCookies := append([]*http.Cookie{}, sessionOnly...)
	postCookies = append(postCookies, formResp.Cookies()...)

	// Build a multipart upload with a zip containing index.html + an asset.
	zipBytes := makeZip(t, map[string]string{
		"index.html": "<!doctype html><p>uploaded page</p>",
		"css/x.css":  "body{}",
	})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("slug", "site")
	_ = mw.WriteField("title", "Site")
	_ = mw.WriteField("status", "published")
	_ = mw.WriteField("visibility", "public")
	_ = mw.WriteField("raw_mode", "on")
	_ = mw.WriteField("_csrf", string(m[1]))
	fw, _ := mw.CreateFormFile("file", "site.zip")
	_, _ = fw.Write(zipBytes)
	require.NoError(t, mw.Close())
	// Build the request manually so the multipart Content-Type is preserved
	// (the do() helper forces urlencoded, which would mislabel the body).
	req := httptest.NewRequest(http.MethodPost, "/admin/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, ck := range postCookies {
		req.AddCookie(ck)
	}
	upload, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, upload.StatusCode)
	require.Equal(t, "/admin", upload.Header.Get("Location"))

	// Public article page serves the raw file (raw_mode=on).
	view, err := do(app, http.MethodGet, "/site", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, view.StatusCode)
	viewBody, _ := io.ReadAll(view.Body)
	require.Contains(t, string(viewBody), "uploaded page")
}

func TestServer_PublicPages_RenderArticle(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)

	// Create a published, public article via the admin form.
	tok := getCSRF(t, app, "/admin/new", cookies)
	form := url.Values{}
	form.Set("slug", "public-post")
	form.Set("title", "Public Post")
	form.Set("body", "# Heading\n\nSome **markdown**.")
	form.Set("status", "published")
	form.Set("visibility", "public")
	form.Set("_csrf", tok)
	_, err := do(app, http.MethodPost, "/admin/new", strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)

	// Public index lists it.
	idx, err := do(app, http.MethodGet, "/", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, idx.StatusCode)
	idxBody, _ := io.ReadAll(idx.Body)
	require.Contains(t, string(idxBody), "Public Post")

	// Public article page renders the markdown.
	art, err := do(app, http.MethodGet, "/public-post", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, art.StatusCode)
	artBody, _ := io.ReadAll(art.Body)
	require.Contains(t, string(artBody), "<h1")
	require.Contains(t, string(artBody), "<strong>markdown</strong>")
}

func TestServer_ProtectedArticle_UnlockFlow(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)

	// Create a published, protected article with no per-article password
	// (falls back to the site default "sitedefault").
	tok := getCSRF(t, app, "/admin/new", cookies)
	form := url.Values{}
	form.Set("slug", "protected-post")
	form.Set("title", "Protected Post")
	form.Set("body", "# Secret\n\nhidden")
	form.Set("status", "published")
	form.Set("visibility", "protected")
	form.Set("_csrf", tok)
	_, err := do(app, http.MethodPost, "/admin/new", strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)

	// Anonymous reader is redirected to the unlock page.
	resp, err := do(app, http.MethodGet, "/protected-post", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/protected-post/unlock", resp.Header.Get("Location"))

	// Fetch the unlock form (carries a fresh CSRF token + cookie).
	unlockResp, err := do(app, http.MethodGet, "/protected-post/unlock", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, unlockResp.StatusCode)
	unlockBody, _ := io.ReadAll(unlockResp.Body)
	m := regexp.MustCompile(`name="_csrf" value="([^"]+)"`).FindSubmatch(unlockBody)
	require.Len(t, m, 2)
	pubCookies := unlockResp.Cookies()

	// Submit the site default password with the CSRF token.
	form = url.Values{}
	form.Set("password", "sitedefault")
	form.Set("_csrf", string(m[1]))
	submit, err := do(app, http.MethodPost, "/protected-post/unlock",
		strings.NewReader(form.Encode()), pubCookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, submit.StatusCode)
	require.Equal(t, "/protected-post", submit.Header.Get("Location"))

	// Collect the unlock cookie and view the now-unlocked article.
	var unlockCookie *http.Cookie
	for _, c := range submit.Cookies() {
		if strings.HasPrefix(c.Name, "wikibuild_unlock_") {
			unlockCookie = c
		}
	}
	require.NotNil(t, unlockCookie)
	view, err := do(app, http.MethodGet, "/protected-post", nil, []*http.Cookie{unlockCookie})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, view.StatusCode)
	viewBody, _ := io.ReadAll(view.Body)
	require.Contains(t, string(viewBody), "hidden")
}

func TestServer_MediaUploadAndServe(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)
	tok := getCSRF(t, app, "/admin/new", cookies)

	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 1, 2, 3, 4, 5}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("_csrf", tok)
	fw, err := mw.CreateFormFile("file", "shot.png")
	require.NoError(t, err)
	_, _ = fw.Write(png)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/admin/media", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), `"/media/`)
	// Extract URL from JSON roughly.
	m := regexp.MustCompile(`"url"\s*:\s*"([^"]+)"`).FindSubmatch(body)
	require.Len(t, m, 2)
	urlPath := string(m[1])

	// Public serve (no auth).
	get, err := do(app, http.MethodGet, urlPath, nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, get.StatusCode)
	got, _ := io.ReadAll(get.Body)
	require.Equal(t, png, got)
}

func TestServer_TagsRenameFlow(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)

	// Create two articles with tags via the admin form.
	tok := getCSRF(t, app, "/admin/new", cookies)
	for _, slug := range []string{"t-a", "t-b"} {
		form := url.Values{}
		form.Set("slug", slug)
		form.Set("title", slug)
		form.Set("body", "x")
		form.Set("tags", "old, keep")
		form.Set("status", "published")
		form.Set("visibility", "public")
		form.Set("_csrf", tok)
		resp, err := do(app, http.MethodPost, "/admin/new",
			strings.NewReader(form.Encode()), cookies)
		require.NoError(t, err)
		require.Equal(t, http.StatusSeeOther, resp.StatusCode)
		tok = getCSRF(t, app, "/admin/new", cookies)
	}

	// Tags page lists them.
	list, err := do(app, http.MethodGet, "/admin/tags", nil, cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, list.StatusCode)
	listBody, _ := io.ReadAll(list.Body)
	require.Contains(t, string(listBody), "old")
	require.Contains(t, string(listBody), "keep")

	// Rename old → new.
	tok = getCSRF(t, app, "/admin/tags", cookies)
	form := url.Values{}
	form.Set("from", "old")
	form.Set("to", "new")
	form.Set("_csrf", tok)
	ren, err := do(app, http.MethodPost, "/admin/tags/rename",
		strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, ren.StatusCode)

	list2, err := do(app, http.MethodGet, "/admin/tags", nil, cookies)
	require.NoError(t, err)
	body2, _ := io.ReadAll(list2.Body)
	require.Contains(t, string(body2), "new")
	require.NotContains(t, string(body2), ">old<")
}

// --- M7: static assets, theme chrome, JSON-LD, v1.0 smoke ---

func TestServer_StaticCSSAndThemeJS(t *testing.T) {
	app := buildApp(t)

	css, err := do(app, http.MethodGet, "/static/css/site.css", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, css.StatusCode)
	body, _ := io.ReadAll(css.Body)
	require.Contains(t, string(body), "--bg")
	require.Contains(t, string(body), "[data-theme=\"dark\"]")

	js, err := do(app, http.MethodGet, "/static/js/theme.js", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, js.StatusCode)
	jsBody, _ := io.ReadAll(js.Body)
	require.Contains(t, string(jsBody), "wikibuild-theme")
}

func TestServer_LayoutIncludesThemeChrome(t *testing.T) {
	app := buildApp(t)
	resp, err := do(app, http.MethodGet, "/", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, `/static/css/site.css`)
	require.Contains(t, s, `/static/js/theme.js`)
	require.Contains(t, s, `data-theme-set="light"`)
	require.Contains(t, s, `data-theme-set="dark"`)
	require.Contains(t, s, `wikibuild-theme`) // FOUC-prevention script
	require.Contains(t, s, `class="site-header"`)
	require.Contains(t, s, `lucide@`)
	require.Contains(t, s, `theme-claude`)
	require.Contains(t, s, `fonts.googleapis.com`)
	require.Contains(t, s, `/static/js/icons.js`)
}

func TestServer_ArticleForm_HasVditorMount(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)
	resp, err := do(app, http.MethodGet, "/admin/new", nil, cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, `id="md-editor"`)
	require.Contains(t, s, `data-editor="vditor"`)
	require.Contains(t, s, `vditor@3.10.9`)
	require.Contains(t, s, `/static/js/editor.js`)
	require.Contains(t, s, `id="body"`)
	require.Contains(t, s, `name="body"`)
}

func TestServer_Article_IncludesJSONLD(t *testing.T) {
	app := buildApp(t)
	cookies := loginSession(t, app)
	tok := getCSRF(t, app, "/admin/new", cookies)
	form := url.Values{}
	form.Set("slug", "jsonld-post")
	form.Set("title", "JSON-LD Post")
	form.Set("body", "Hello structured data world with enough text for a description.")
	form.Set("status", "published")
	form.Set("visibility", "public")
	form.Set("_csrf", tok)
	_, err := do(app, http.MethodPost, "/admin/new", strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)

	resp, err := do(app, http.MethodGet, "/jsonld-post", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, `application/ld+json`)
	require.Contains(t, s, `BlogPosting`)
	require.Contains(t, s, `https://ex.com/jsonld-post`)
	require.Contains(t, s, `JSON-LD Post`)
}

func TestServer_V1Smoke_PublicSurface(t *testing.T) {
	// Acceptance: core public endpoints respond and feeds/discovery work together.
	app := buildApp(t)
	cookies := loginSession(t, app)
	tok := getCSRF(t, app, "/admin/new", cookies)
	form := url.Values{}
	form.Set("slug", "smoke")
	form.Set("title", "Smoke Test")
	form.Set("body", "Body for smoke # Heading\n\nParagraph.")
	form.Set("tags", "smoke")
	form.Set("status", "published")
	form.Set("visibility", "public")
	form.Set("_csrf", tok)
	resp, err := do(app, http.MethodPost, "/admin/new", strings.NewReader(form.Encode()), cookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	checks := []struct {
		path string
		want string
	}{
		{"/", "Smoke Test"},
		{"/smoke", "Smoke Test"},
		{"/search?q=Smoke", "Smoke Test"},
		{"/tag/smoke", "Smoke Test"},
		{"/archive", "封存"},
		{"/feed", "Smoke Test"},
		{"/feed/atom", "Smoke Test"},
		{"/feed.json", "Smoke Test"},
		{"/sitemap.xml", "/smoke"},
		{"/robots.txt", "Sitemap:"},
	}
	for _, tc := range checks {
		r, err := do(app, http.MethodGet, tc.path, nil, nil)
		require.NoError(t, err, tc.path)
		require.Equal(t, http.StatusOK, r.StatusCode, tc.path)
		b, _ := io.ReadAll(r.Body)
		require.Contains(t, string(b), tc.want, tc.path)
	}
}
