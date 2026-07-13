package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// fakeHasher mirrors bcrypt's contract without its cost, so handler tests
// stay fast. Hash is "H:"+password; Compare matches accordingly.
type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "H:" + p, nil }
func (fakeHasher) Compare(h, p string) error {
	if h == "H:"+p {
		return nil
	}
	return auth.ErrPasswordMismatch
}

const (
	testUser = "admin"
	testPass = "s3cret"
)

// buildApp wires an AdminAuth handler onto a fresh Fiber app with an inmem
// store seeded with one user, returning the app and the limiter's fake clock.
func buildApp(t *testing.T) (*fiber.App, *clock.Fake) {
	t.Helper()
	repo := inmem.New()
	_, err := repo.CreateUser(t.Context(), model.User{Username: testUser, PasswordHash: "H:" + testPass})
	require.NoError(t, err)

	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	signer := auth.NewSigner("supersecretkey1234", fc)
	limiter := auth.NewLoginLimiter(fc, auth.LimiterConfig{
		MaxAttempts: 3, Window: 10 * time.Minute, Lockout: 15 * time.Minute,
	})

	h := handler.NewAdminAuth(handler.AdminAuthDeps{
		Store:   repo,
		Hasher:  fakeHasher{},
		Signer:  signer,
		Limiter: limiter,
	})

	app := fiber.New()
	app.Get("/admin/login", h.LoginPage)
	app.Post("/admin/login", h.LoginSubmit)
	app.Post("/admin/logout", h.Logout)
	app.Get("/admin", h.RequireAuth, h.Dashboard)
	return app, fc
}

func postForm(app *fiber.App, path, username, password string) *http.Response {
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := app.Test(req)
	return resp
}

func TestLoginPage_OK(t *testing.T) {
	app, _ := buildApp(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "password")
}

func TestLoginSubmit_CorrectCredentials_RedirectsAndSetsCookie(t *testing.T) {
	app, _ := buildApp(t)
	resp := postForm(app, "/admin/login", testUser, testPass)

	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/admin", resp.Header.Get("Location"))
	require.NotEmpty(t, resp.Header.Get("Set-Cookie"))
}

func TestLoginSubmit_WrongPassword_Unauthorized(t *testing.T) {
	app, _ := buildApp(t)
	resp := postForm(app, "/admin/login", testUser, "wrong")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLoginSubmit_UnknownUser_Unauthorized(t *testing.T) {
	app, _ := buildApp(t)
	resp := postForm(app, "/admin/login", "nobody", testPass)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLoginSubmit_LocksAfterMaxAttempts(t *testing.T) {
	app, _ := buildApp(t)
	// Limiter config: MaxAttempts=3
	for i := 0; i < 3; i++ {
		resp := postForm(app, "/admin/login", testUser, "wrong")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	// 4th attempt: now locked.
	resp := postForm(app, "/admin/login", testUser, "wrong")
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestLogout_ClearsCookie(t *testing.T) {
	app, _ := buildApp(t)
	resp := postForm(app, "/admin/logout", "", "")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	// Set-Cookie must clear the session cookie (Max-Age=0 / empty value).
	cookie := resp.Header.Get("Set-Cookie")
	require.Contains(t, cookie, "wikibuild_admin=")
}

func TestRequireAuth_NoCookie_RedirectsToLogin(t *testing.T) {
	app, _ := buildApp(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/admin/login", resp.Header.Get("Location"))
}

func TestRequireAuth_ValidCookie_AllowsAccess(t *testing.T) {
	app, fc := buildApp(t)
	signer := auth.NewSigner("supersecretkey1234", fc)
	tok, err := signer.Sign(testUser, time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "wikibuild_admin", Value: tok})
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), testUser)
}

func TestRequireAuth_ExpiredCookie_RedirectsToLogin(t *testing.T) {
	app, fc := buildApp(t)
	signer := auth.NewSigner("supersecretkey1234", fc)
	tok, err := signer.Sign(testUser, time.Hour)
	require.NoError(t, err)

	// Advance past the session TTL.
	fc.Set(time.Unix(1_700_000_000, 0).Add(2 * time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "wikibuild_admin", Value: tok})
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
}
