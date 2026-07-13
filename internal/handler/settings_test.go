package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func settingsApp(t *testing.T) (*fiber.App, store.Repository) {
	t.Helper()
	repo := inmem.New()
	s := handler.NewSettings(repo)
	app := fiber.New()
	app.Get("/admin/settings", s.Form)
	app.Post("/admin/settings", s.Save)
	return app, repo
}

func TestSettings_Form_OK(t *testing.T) {
	app, _ := settingsApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/settings", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSettings_Save_PersistsValue(t *testing.T) {
	app, repo := settingsApp(t)
	form := url.Values{}
	form.Set("default_protected_password", "newdefault")
	req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/admin/settings", resp.Header.Get("Location"))

	got, err := repo.GetSetting(nil, "default_protected_password")
	require.NoError(t, err)
	require.Equal(t, "newdefault", got)
}

func TestSettings_FormShowsStoredValue(t *testing.T) {
	app, repo := settingsApp(t)
	require.NoError(t, repo.SetSetting(nil, "default_protected_password", "sitedefault"))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/settings", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), `value="sitedefault"`)
}

func TestSettings_Save_EmptyClearsValue(t *testing.T) {
	app, repo := settingsApp(t)
	require.NoError(t, repo.SetSetting(nil, "default_protected_password", "temp"))

	form := url.Values{}
	form.Set("default_protected_password", "")
	req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := app.Test(req)
	require.NoError(t, err)

	got, err := repo.GetSetting(nil, "default_protected_password")
	require.NoError(t, err)
	require.Empty(t, got, "empty submission clears the setting (env fallback applies)")
}
