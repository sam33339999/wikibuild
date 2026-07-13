package handler

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Settings manages site-wide settings (currently the default protected
// password). It depends only on store.Repository so it is unit-tested against
// inmem.
type Settings struct {
	repo store.Repository
}

// NewSettings builds a Settings handler.
func NewSettings(repo store.Repository) *Settings {
	return &Settings{repo: repo}
}

// Form renders the settings page with the current default protected password.
func (h *Settings) Form(c fiber.Ctx) error {
	current, err := h.repo.GetSetting(c.Context(), SettingDefaultProtectedPass)
	if err != nil {
		return err
	}
	return renderPage(c, "設定", adminviews.Settings(current, csrf.TokenFromContext(c)))
}

// Save updates the default protected password.
func (h *Settings) Save(c fiber.Ctx) error {
	// Clone: fasthttp reuses request buffers, so values stored beyond the
	// request must be copied.
	value := strings.Clone(c.FormValue("default_protected_password"))
	if err := h.repo.SetSetting(c.Context(), SettingDefaultProtectedPass, value); err != nil {
		return err
	}
	return c.Redirect().To("/admin/settings")
}
