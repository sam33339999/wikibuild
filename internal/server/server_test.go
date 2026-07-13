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
		Store:   repo,
		Hasher:  fakeHasher{},
		Signer:  auth.NewSigner("supersecretkey1234", fc),
		Limiter: auth.NewLoginLimiter(fc, auth.DefaultLimiterConfig()),
		Clock:   fc,
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
