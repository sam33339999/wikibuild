package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/sam33339999/wikibuild/internal/store"
	publicviews "github.com/sam33339999/wikibuild/views/public"
)

// defaultPageSize is the article count per homepage page.
const defaultPageSize = 10

// Public serves reader-facing pages: the paginated article index and single
// article pages with rendered markdown. Visibility gating (protected/private)
// is added in M2; for M1 only published+public articles are exposed.
type Public struct {
	repo     store.Repository
	pageSize int
}

// NewPublic builds a Public handler with the default page size.
func NewPublic(repo store.Repository) *Public {
	return &Public{repo: repo, pageSize: defaultPageSize}
}

// Index renders the paginated list of published, public articles.
func (h *Public) Index(c fiber.Ctx) error {
	page := parsePage(c.Query("page"))
	offset := (page - 1) * h.pageSize

	items, total, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
		Limit:      h.pageSize,
		Offset:     offset,
	})
	if err != nil {
		return err
	}
	totalPages := (total + h.pageSize - 1) / h.pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	return renderPage(c, "文章", publicviews.Index(items, page, totalPages))
}

// Article renders a single article by slug. Drafts and non-public visibility
// return 404 (M2 refines private/protected handling).
func (h *Public) Article(c fiber.Ctx) error {
	a, err := h.repo.GetArticleBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	if a.Status != model.StatusPublished || a.Visibility != model.VisibilityPublic {
		return c.SendStatus(http.StatusNotFound)
	}

	html, toc := render.RenderWithTOC(a.Body)
	return renderPage(c, a.Title, publicviews.Article(a, html, toc))
}

// parsePage reads a ?page query param, clamped to >= 1. Bad input defaults to 1.
func parsePage(s string) int {
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}
