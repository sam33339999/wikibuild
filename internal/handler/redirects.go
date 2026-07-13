package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Redirects manages permanent path redirects (slug renames + manual entries).
type Redirects struct {
	repo store.Repository
}

// NewRedirects builds a Redirects admin handler.
func NewRedirects(repo store.Repository) *Redirects {
	return &Redirects{repo: repo}
}

// List renders the redirect management page.
func (h *Redirects) List(c fiber.Ctx) error {
	items, err := h.repo.ListRedirects(c.Context())
	if err != nil {
		return err
	}
	return renderPage(c, "導向管理", adminviews.RedirectList(items, csrf.TokenFromContext(c)))
}

// Create adds or updates a redirect from the form.
func (h *Redirects) Create(c fiber.Ctx) error {
	from := normalizePath(strings.Clone(c.FormValue("from_path")))
	to := normalizePath(strings.Clone(c.FormValue("to_path")))
	if from == "" || to == "" {
		return c.Status(http.StatusBadRequest).SendString("from_path and to_path required")
	}
	if _, err := h.repo.CreateRedirect(c.Context(), model.Redirect{
		FromPath: from,
		ToPath:   to,
	}); err != nil {
		return err
	}
	return c.Redirect().To("/admin/redirects")
}

// Delete removes a redirect by from_path form field.
func (h *Redirects) Delete(c fiber.Ctx) error {
	from := normalizePath(strings.Clone(c.FormValue("from_path")))
	if err := h.repo.DeleteRedirect(c.Context(), from); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	return c.Redirect().To("/admin/redirects")
}

// normalizePath ensures a leading slash and strips trailing slash (except root).
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

// tryRedirect looks up a 301 for path (e.g. /old-slug). Returns true if a
// redirect response was written.
func tryRedirect(c fiber.Ctx, repo store.Repository, path string) (bool, error) {
	path = normalizePath(path)
	r, err := repo.GetRedirect(c.Context(), path)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, c.Redirect().Status(http.StatusMovedPermanently).To(r.ToPath)
}
