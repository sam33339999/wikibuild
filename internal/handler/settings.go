package handler

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/sitebrand"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Settings manages site-wide brand, privacy, and comment settings.
type Settings struct {
	repo store.Repository
}

// NewSettings builds a Settings handler.
func NewSettings(repo store.Repository) *Settings {
	return &Settings{repo: repo}
}

// Form renders the settings page.
func (h *Settings) Form(c fiber.Ctx) error {
	data := adminviews.SettingsData{
		SiteName:             getSetting(h.repo, c, sitebrand.KeyName),
		SiteTagline:          getSetting(h.repo, c, sitebrand.KeyTagline),
		AuthorName:           getSetting(h.repo, c, sitebrand.KeyAuthor),
		AuthorBio:            getSetting(h.repo, c, sitebrand.KeyBio),
		SocialGitHub:         getSetting(h.repo, c, sitebrand.KeyGitHub),
		SocialX:              getSetting(h.repo, c, sitebrand.KeyX),
		SocialEmail:          getSetting(h.repo, c, sitebrand.KeyEmail),
		DefaultProtectedPass: getSetting(h.repo, c, SettingDefaultProtectedPass),
		CommentProvider:      getSetting(h.repo, c, SettingCommentProvider),
		CommentRepo:          getSetting(h.repo, c, SettingCommentRepo),
		CommentCategory:      getSetting(h.repo, c, SettingCommentCategory),
	}
	return renderPage(c, "設定", adminviews.Settings(data, csrf.TokenFromContext(c)))
}

// Save updates site-wide settings from the form.
func (h *Settings) Save(c fiber.Ctx) error {
	pairs := []struct{ key, val string }{
		{sitebrand.KeyName, strings.Clone(strings.TrimSpace(c.FormValue("site_name")))},
		{sitebrand.KeyTagline, strings.Clone(strings.TrimSpace(c.FormValue("site_tagline")))},
		{sitebrand.KeyAuthor, strings.Clone(strings.TrimSpace(c.FormValue("author_name")))},
		{sitebrand.KeyBio, strings.Clone(c.FormValue("author_bio"))},
		{sitebrand.KeyGitHub, strings.Clone(strings.TrimSpace(c.FormValue("social_github")))},
		{sitebrand.KeyX, strings.Clone(strings.TrimSpace(c.FormValue("social_x")))},
		{sitebrand.KeyEmail, strings.Clone(strings.TrimSpace(c.FormValue("social_email")))},
		{SettingDefaultProtectedPass, strings.Clone(c.FormValue("default_protected_password"))},
		{SettingCommentProvider, strings.Clone(c.FormValue("comment_provider"))},
		{SettingCommentRepo, strings.Clone(strings.TrimSpace(c.FormValue("comment_repo")))},
		{SettingCommentCategory, strings.Clone(strings.TrimSpace(c.FormValue("comment_category")))},
	}
	for _, p := range pairs {
		if err := h.repo.SetSetting(c.Context(), p.key, p.val); err != nil {
			return err
		}
	}
	return c.Redirect().To("/admin/settings")
}

func getSetting(repo store.Repository, c fiber.Ctx, key string) string {
	v, _ := repo.GetSetting(c.Context(), key)
	return v
}
