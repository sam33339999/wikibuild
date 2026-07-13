// Package sitebrand loads the public-facing site identity from settings.
package sitebrand

import (
	"context"
	"strings"

	"github.com/sam33339999/wikibuild/internal/store"
)

// Setting keys for brand fields (settings table).
const (
	KeyName     = "site_name"
	KeyTagline  = "site_tagline"
	KeyAuthor   = "author_name"
	KeyBio      = "author_bio"
	KeyGitHub   = "social_github"
	KeyX        = "social_x"
	KeyEmail    = "social_email"
)

// Brand is the public identity of the site (engineering notebook + portfolio).
type Brand struct {
	Name    string // site title in header / <title> suffix
	Tagline string // short line under name
	Author  string // display name
	Bio     string // short intro for homepage
	GitHub  string // owner or full URL
	X       string // handle without @ or URL
	Email   string // mailto address
}

// Default returns a fallback brand when settings are empty.
func Default(fallbackName string) Brand {
	if fallbackName == "" {
		fallbackName = "WikiBuild"
	}
	return Brand{
		Name:    fallbackName,
		Tagline: "Engineering notes & selected work",
		Author:  "",
		Bio:     "",
	}
}

// Load reads brand fields from the repository. Missing keys use defaults.
// fallbackName is used when site_name is unset (e.g. WIKIBUILD_SITE_TITLE).
func Load(ctx context.Context, repo store.Repository, fallbackName string) Brand {
	b := Default(fallbackName)
	if repo == nil {
		return b
	}
	if v, _ := repo.GetSetting(ctx, KeyName); strings.TrimSpace(v) != "" {
		b.Name = strings.TrimSpace(v)
	}
	if v, _ := repo.GetSetting(ctx, KeyTagline); v != "" {
		b.Tagline = strings.TrimSpace(v)
	}
	if v, _ := repo.GetSetting(ctx, KeyAuthor); v != "" {
		b.Author = strings.TrimSpace(v)
	}
	if v, _ := repo.GetSetting(ctx, KeyBio); v != "" {
		b.Bio = strings.TrimSpace(v)
	}
	if v, _ := repo.GetSetting(ctx, KeyGitHub); v != "" {
		b.GitHub = strings.TrimSpace(v)
	}
	if v, _ := repo.GetSetting(ctx, KeyX); v != "" {
		b.X = strings.TrimSpace(v)
	}
	if v, _ := repo.GetSetting(ctx, KeyEmail); v != "" {
		b.Email = strings.TrimSpace(v)
	}
	return b
}

// GitHubURL normalizes a github field into an https URL.
func (b Brand) GitHubURL() string {
	g := strings.TrimSpace(b.GitHub)
	if g == "" {
		return ""
	}
	if strings.HasPrefix(g, "http://") || strings.HasPrefix(g, "https://") {
		return g
	}
	return "https://github.com/" + strings.TrimPrefix(g, "@")
}

// XURL normalizes an X/Twitter handle into an https URL.
func (b Brand) XURL() string {
	x := strings.TrimSpace(b.X)
	if x == "" {
		return ""
	}
	if strings.HasPrefix(x, "http://") || strings.HasPrefix(x, "https://") {
		return x
	}
	return "https://x.com/" + strings.TrimPrefix(x, "@")
}

// Mailto returns a mailto: link or empty.
func (b Brand) Mailto() string {
	e := strings.TrimSpace(b.Email)
	if e == "" {
		return ""
	}
	if strings.HasPrefix(e, "mailto:") {
		return e
	}
	return "mailto:" + e
}
