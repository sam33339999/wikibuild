package handler

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Settings manages site-wide settings (protected password + comments).
type Settings struct {
	repo store.Repository
}

// NewSettings builds a Settings handler.
func NewSettings(repo store.Repository) *Settings {
	return &Settings{repo: repo}
}

// Form renders the settings page.
func (h *Settings) Form(c fiber.Ctx) error {
	pass, err := h.repo.GetSetting(c.Context(), SettingDefaultProtectedPass)
	if err != nil {
		return err
	}
	provider, _ := h.repo.GetSetting(c.Context(), SettingCommentProvider)
	repo, _ := h.repo.GetSetting(c.Context(), SettingCommentRepo)
	cat, _ := h.repo.GetSetting(c.Context(), SettingCommentCategory)
	data := adminviews.SettingsData{
		DefaultProtectedPass: pass,
		CommentProvider:      provider,
		CommentRepo:          repo,
		CommentCategory:      cat,
	}
	return renderPage(c, "設定", adminviews.Settings(data, csrf.TokenFromContext(c)))
}

// Save updates site-wide settings from the form.
func (h *Settings) Save(c fiber.Ctx) error {
	// Clone: fasthttp reuses request buffers.
	pass := strings.Clone(c.FormValue("default_protected_password"))
	if err := h.repo.SetSetting(c.Context(), SettingDefaultProtectedPass, pass); err != nil {
		return err
	}
	if err := h.repo.SetSetting(c.Context(), SettingCommentProvider, strings.Clone(c.FormValue("comment_provider"))); err != nil {
		return err
	}
	if err := h.repo.SetSetting(c.Context(), SettingCommentRepo, strings.Clone(strings.TrimSpace(c.FormValue("comment_repo")))); err != nil {
		return err
	}
	if err := h.repo.SetSetting(c.Context(), SettingCommentCategory, strings.Clone(strings.TrimSpace(c.FormValue("comment_category")))); err != nil {
		return err
	}
	return c.Redirect().To("/admin/settings")
}
