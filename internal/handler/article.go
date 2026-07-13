package handler

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
	"github.com/sam33339999/wikibuild/views/layout"
)

// ArticleAdmin handles admin article CRUD. It depends only on store.Repository
// so it is unit-tested against inmem. Markdown rendering happens on the public
// side (M1.3); admin forms store raw markdown.
type ArticleAdmin struct {
	repo store.Repository
}

// NewArticleAdmin builds an ArticleAdmin backed by the given repository.
func NewArticleAdmin(repo store.Repository) *ArticleAdmin {
	return &ArticleAdmin{repo: repo}
}

// List shows every article (newest first) for the admin overview.
func (h *ArticleAdmin) List(c fiber.Ctx) error {
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{})
	if err != nil {
		return err
	}
	return renderPage(c, "文章列表", adminviews.ArticleList(items))
}

// NewForm renders a blank article form.
func (h *ArticleAdmin) NewForm(c fiber.Ctx) error {
	return renderPage(c, "新增文章", adminviews.ArticleForm("/admin/new", &model.Article{
		Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	}, csrf.TokenFromContext(c)))
}

// Create stores a new markdown article from the form.
func (h *ArticleAdmin) Create(c fiber.Ctx) error {
	a := articleFromForm(c)
	a.Type = model.ArticleTypeMarkdown

	created, err := h.repo.CreateArticle(c.Context(), a)
	if err != nil {
		return articleCreateErr(c, err)
	}
	return c.Redirect().To("/admin/" + strconv.FormatInt(created.ID, 10) + "/edit")
}

// EditForm renders the article form pre-filled for editing.
func (h *ArticleAdmin) EditForm(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	a, err := h.repo.GetArticle(c.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	return renderPage(c, "編輯："+a.Title, adminviews.ArticleForm(
		"/admin/"+strconv.FormatInt(a.ID, 10), &a, csrf.TokenFromContext(c)))
}

// Update applies form edits to an existing article.
func (h *ArticleAdmin) Update(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	existing, err := h.repo.GetArticle(c.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}

	updated := articleFromForm(c)
	updated.ID = existing.ID
	updated.Type = existing.Type // type is immutable in M1 (markdown; M3 adds html_upload)
	updated.CreatedAt = existing.CreatedAt

	if _, err := h.repo.UpdateArticle(c.Context(), updated); err != nil {
		if errors.Is(err, store.ErrDuplicateSlug) {
			return c.Status(http.StatusConflict).SendString("slug already exists")
		}
		return err
	}
	return c.Redirect().To("/admin")
}

// Delete removes an article.
func (h *ArticleAdmin) Delete(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	if err := h.repo.DeleteArticle(c.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	return c.Redirect().To("/admin")
}

// articleFromForm reads the shared form fields into a model.Article. Used by
// both Create and Update so the field set stays in one place.
func articleFromForm(c fiber.Ctx) model.Article {
	return model.Article{
		Slug:       strings.TrimSpace(c.FormValue("slug")),
		Title:      strings.TrimSpace(c.FormValue("title")),
		Body:       c.FormValue("body"),
		Tags:       parseTags(c.FormValue("tags")),
		Status:     model.Status(c.FormValue("status")),
		Visibility: model.Visibility(c.FormValue("visibility")),
	}
}

// parseTags splits a comma-separated tag string, trimming whitespace and
// dropping empties.
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// articleCreateErr maps CreateArticle errors to HTTP responses.
func articleCreateErr(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, store.ErrEmptySlug):
		return c.SendStatus(http.StatusBadRequest)
	case errors.Is(err, store.ErrDuplicateSlug):
		return c.Status(http.StatusConflict).SendString("slug already exists")
	default:
		return err
	}
}

// renderPage wraps a templ component in the shared layout and writes it to the
// response. Centralised so handlers stay one line.
func renderPage(c fiber.Ctx, title string, comp templ.Component) error {
	var buf bytes.Buffer
	if err := layout.Page(title, comp).Render(c.Context(), &buf); err != nil {
		return err
	}
	return c.Type("html").Send(buf.Bytes())
}
