package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	app := server.New(server.Deps{
		Store:           repo,
		Hasher:          fakeHasher{},
		Signer:          auth.NewSigner("supersecretkey1234", fc),
		Limiter:         auth.NewLoginLimiter(fc, auth.DefaultLimiterConfig()),
		Clock:           fc,
		SiteDefaultPass: "sitedefault",
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
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "missing csrf token must be 403")
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
