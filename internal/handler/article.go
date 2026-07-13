package handler

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
	"github.com/sam33339999/wikibuild/views/layout"
)

// ArticleAdmin handles admin article CRUD. It depends only on store.Repository
// and auth.PasswordHasher so it is unit-tested against inmem with a fake
// hasher. The protected-article password field is bcrypt-hashed on save.
// clock stamps PublishedAt when an article first becomes published.
type ArticleAdmin struct {
	repo   store.Repository
	hasher auth.PasswordHasher
	clock  clock.Clock
}

// NewArticleAdmin builds an ArticleAdmin backed by the given repository.
// hasher hashes per-article protected passwords. clk may be nil (falls back
// to clock.Real) for older call sites/tests.
func NewArticleAdmin(repo store.Repository, hasher auth.PasswordHasher, clk clock.Clock) *ArticleAdmin {
	if clk == nil {
		clk = clock.Real{}
	}
	return &ArticleAdmin{repo: repo, hasher: hasher, clock: clk}
}

// List shows every article (newest first) for the admin overview.
// Optional ?q= filters title/body via store search.
func (h *ArticleAdmin) List(c fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{Search: q})
	if err != nil {
		return err
	}
	return renderPage(c, "文章列表", adminviews.ArticleList(items, q))
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
	a.Password = hashPasswordIfSet(c, h.hasher)
	stampPublishedAt(&a, nil, h.clock.Now())
	if err := ensurePreviewToken(&a, nil, c.FormValue("regen_preview") == "on"); err != nil {
		return err
	}

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
	// Empty password on edit means "keep current"; a non-empty value re-hashes.
	updated.Password = keepOrHashPassword(c, h.hasher, existing.Password)
	stampPublishedAt(&updated, &existing, h.clock.Now())
	if err := ensurePreviewToken(&updated, &existing, c.FormValue("regen_preview") == "on"); err != nil {
		return err
	}

	if _, err := h.repo.UpdateArticle(c.Context(), updated); err != nil {
		if errors.Is(err, store.ErrDuplicateSlug) {
			return c.Status(http.StatusConflict).SendString("slug already exists")
		}
		return err
	}
	// Slug rename → permanent redirect from the old path.
	if existing.Slug != updated.Slug {
		_, _ = h.repo.CreateRedirect(c.Context(), model.Redirect{
			FromPath: normalizePath(existing.Slug),
			ToPath:   normalizePath(updated.Slug),
		})
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
// both Create and Update so the field set stays in one place. The protected
// password is handled separately (see hashPasswordIfSet/keepOrHashPassword)
// because its edit semantics differ (blank = keep current).
//
// Form values are cloned: Fiber/fasthttp backs c.FormValue strings with a
// per-request buffer that is reused after the handler returns, so anything
// stored beyond the request (in the DB) must be copied.
func articleFromForm(c fiber.Ctx) model.Article {
	a := model.Article{
		Slug:       strings.Clone(strings.TrimSpace(c.FormValue("slug"))),
		Title:      strings.Clone(strings.TrimSpace(c.FormValue("title"))),
		Body:       strings.Clone(c.FormValue("body")),
		Tags:       parseTags(c.FormValue("tags")),
		Status:     model.Status(strings.Clone(c.FormValue("status"))),
		Visibility: model.Visibility(strings.Clone(c.FormValue("visibility"))),
		Pinned:     c.FormValue("pinned") == "on",
	}
	if raw := strings.TrimSpace(c.FormValue("publish_at")); raw != "" {
		if t, err := parseFormTime(raw); err == nil {
			a.PublishAt = &t
		}
	}
	return a
}

// parseFormTime accepts datetime-local (2006-01-02T15:04) or RFC3339.
func parseFormTime(s string) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}

// ensurePreviewToken keeps or issues a draft preview token. Published articles
// may keep a token (still works) but drafts always get one.
func ensurePreviewToken(a *model.Article, existing *model.Article, regen bool) error {
	if existing != nil && existing.PreviewToken != "" && !regen {
		a.PreviewToken = existing.PreviewToken
		return nil
	}
	if a.PreviewToken != "" && !regen {
		return nil
	}
	tok, err := newPreviewToken()
	if err != nil {
		return err
	}
	a.PreviewToken = tok
	return nil
}

// hashPasswordIfSet hashes a non-empty "password" form field for new articles;
// an empty field leaves the article with no article-specific password (the
// site default applies).
func hashPasswordIfSet(c fiber.Ctx, h auth.PasswordHasher) string {
	pw := strings.Clone(c.FormValue("password"))
	if pw == "" {
		return ""
	}
	hash, err := h.Hash(pw)
	if err != nil {
		return ""
	}
	return hash
}

// keepOrHashPassword hashes a non-empty "password" field on edit; an empty
// field preserves the existing hash so editing other fields doesn't clear it.
func keepOrHashPassword(c fiber.Ctx, h auth.PasswordHasher, existing string) string {
	pw := strings.Clone(c.FormValue("password"))
	if pw == "" {
		return existing
	}
	hash, err := h.Hash(pw)
	if err != nil {
		return existing
	}
	return hash
}

// stampPublishedAt sets PublishedAt the first time status becomes published.
// Existing timestamps are preserved on re-save; drafts clear PublishedAt.
// Scheduled drafts keep PublishAt; publishing clears the schedule.
func stampPublishedAt(a *model.Article, existing *model.Article, now time.Time) {
	if a.Status != model.StatusPublished {
		a.PublishedAt = nil
		// Keep a.PublishAt from the form (schedule).
		return
	}
	// Immediately published: drop schedule.
	a.PublishAt = nil
	if existing != nil && existing.PublishedAt != nil {
		a.PublishedAt = existing.PublishedAt
		return
	}
	t := now.UTC()
	a.PublishedAt = &t
}

// parseTags splits a comma-separated tag string, trimming whitespace and
// dropping empties. Each tag is cloned (fasthttp reuses form buffers).
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, strings.Clone(t))
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
	return renderPageSEO(c, title, comp, layout.SEO{})
}

// renderPageSEO is like renderPage but attaches Open Graph / canonical meta.
func renderPageSEO(c fiber.Ctx, title string, comp templ.Component, seo layout.SEO) error {
	var buf bytes.Buffer
	if err := layout.Page(title, comp, seo).Render(c.Context(), &buf); err != nil {
		return err
	}
	return c.Type("html").Send(buf.Bytes())
}
