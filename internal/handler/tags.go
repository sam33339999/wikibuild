package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Tags handles admin tag management: list, rename, and merge (merge is
// rename-when-target-exists). Depends only on store.Repository.
type Tags struct {
	repo store.Repository
}

// NewTags builds a Tags handler.
func NewTags(repo store.Repository) *Tags {
	return &Tags{repo: repo}
}

// List renders the tag management page with every distinct tag and count.
func (h *Tags) List(c fiber.Ctx) error {
	tags, err := h.repo.ListTags(c.Context())
	if err != nil {
		return err
	}
	return renderPage(c, "標籤管理", adminviews.TagList(tags, csrf.TokenFromContext(c), ""))
}

// Rename renames a tag across all articles (form fields: from, to). When
// to already exists on an article, from is merged away without duplicates.
func (h *Tags) Rename(c fiber.Ctx) error {
	from := strings.Clone(strings.TrimSpace(c.FormValue("from")))
	to := strings.Clone(strings.TrimSpace(c.FormValue("to")))
	if from == "" || to == "" {
		return c.Status(http.StatusBadRequest).SendString("from and to are required")
	}
	n, err := h.repo.RenameTag(c.Context(), from, to)
	if err != nil {
		if errors.Is(err, store.ErrEmptyTag) {
			return c.Status(http.StatusBadRequest).SendString("empty tag")
		}
		return err
	}
	_ = n
	return c.Redirect().To("/admin/tags")
}

// Merge is an alias of Rename with clearer form field names (from, into)
// for the merge UI. Behaviour is identical: from→into with dedupe.
func (h *Tags) Merge(c fiber.Ctx) error {
	from := strings.Clone(strings.TrimSpace(c.FormValue("from")))
	into := strings.Clone(strings.TrimSpace(c.FormValue("into")))
	if from == "" || into == "" {
		return c.Status(http.StatusBadRequest).SendString("from and into are required")
	}
	if _, err := h.repo.RenameTag(c.Context(), from, into); err != nil {
		if errors.Is(err, store.ErrEmptyTag) {
			return c.Status(http.StatusBadRequest).SendString("empty tag")
		}
		return err
	}
	return c.Redirect().To("/admin/tags")
}
