package handler

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/sitebrand"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
	"github.com/sam33339999/wikibuild/views/layout"
)

// Locals key for the request-scoped site brand (set by server middleware).
const localsBrandKey = "siteBrand"

// ArticleAdmin handles admin article CRUD. It depends only on store.Repository
// and auth.PasswordHasher so it is unit-tested against inmem with a fake
// hasher. The protected-article password field is bcrypt-hashed on save.
// clock stamps PublishedAt when an article first becomes published.
// contentDir is used to remove html_upload files on delete (may be empty in tests).
// llmEnabled toggles the admin "AI 產生 SEO" control (S2).
type ArticleAdmin struct {
	repo       store.Repository
	hasher     auth.PasswordHasher
	clock      clock.Clock
	contentDir string
	llmEnabled bool
}

// NewArticleAdmin builds an ArticleAdmin backed by the given repository.
// hasher hashes per-article protected passwords. clk may be nil (falls back
// to clock.Real) for older call sites/tests. contentDir may be empty.
// llmEnabled shows the AI SEO button when the LLM client is configured.
func NewArticleAdmin(repo store.Repository, hasher auth.PasswordHasher, clk clock.Clock, contentDir string, llmEnabled bool) *ArticleAdmin {
	if clk == nil {
		clk = clock.Real{}
	}
	return &ArticleAdmin{repo: repo, hasher: hasher, clock: clk, contentDir: contentDir, llmEnabled: llmEnabled}
}

// List shows every article (newest first) for the admin overview.
// Optional ?q= filters title/body via store search.
func (h *ArticleAdmin) List(c fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{Search: q})
	if err != nil {
		return err
	}
	return renderPage(c, "文章列表", adminviews.ArticleList(items, q, csrf.TokenFromContext(c)))
}

// editorSearchLimit caps JSON results for the writing-time search panel (S3a).
const editorSearchLimit = 20

// SearchJSON serves GET /admin/api/articles/search?q=&exclude_id= for the
// markdown editor link panel. Returns admin-visible articles (any status /
// visibility). Does not leak via public routes — caller must be behind RequireAuth.
func (h *ArticleAdmin) SearchJSON(c fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.JSON([]any{})
	}
	excludeID, _ := strconv.ParseInt(c.Query("exclude_id"), 10, 64)

	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Search: q,
		Limit:  editorSearchLimit + 5, // fetch a few extra if excluding self
	})
	if err != nil {
		return err
	}

	type hit struct {
		ID         int64  `json:"id"`
		Slug       string `json:"slug"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		Visibility string `json:"visibility"`
	}
	out := make([]hit, 0, editorSearchLimit)
	for _, a := range items {
		if excludeID > 0 && a.ID == excludeID {
			continue
		}
		out = append(out, hit{
			ID:         a.ID,
			Slug:       a.Slug,
			Title:      a.Title,
			Status:     string(a.Status),
			Visibility: string(a.Visibility),
		})
		if len(out) >= editorSearchLimit {
			break
		}
	}
	return c.JSON(out)
}

// NewForm renders a blank article form.
func (h *ArticleAdmin) NewForm(c fiber.Ctx) error {
	return renderPage(c, "新增文章", adminviews.ArticleForm("/admin/new", &model.Article{
		Status: model.StatusDraft, Visibility: model.VisibilityPublic, ShowTOC: true,
	}, csrf.TokenFromContext(c), h.llmEnabled))
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
// Markdown uses the full editor; html_upload uses a metadata-only form
// (body is a filesystem path, not markdown).
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
	tok := csrf.TokenFromContext(c)
	action := "/admin/" + strconv.FormatInt(a.ID, 10)
	if a.Type == model.ArticleTypeHTMLUpload {
		return renderPage(c, "編輯上傳："+a.Title, adminviews.HTMLUploadEdit(action, &a, tok, h.llmEnabled))
	}
	return renderPage(c, "編輯："+a.Title, adminviews.ArticleForm(action, &a, tok, h.llmEnabled))
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

	now := h.clock.Now()
	var updated model.Article
	if existing.Type == model.ArticleTypeHTMLUpload {
		// Metadata only — never overwrite Body (entry file path) from a markdown form.
		updated = htmlUploadFromForm(c, existing, h.hasher)
		stampPublishedAt(&updated, &existing, now)
	} else {
		updated = articleFromForm(c)
		updated.ID = existing.ID
		updated.Type = existing.Type
		updated.CreatedAt = existing.CreatedAt
		updated.Password = keepOrHashPassword(c, h.hasher, existing.Password)
		updated.PreviewToken = existing.PreviewToken
		stampPublishedAt(&updated, &existing, now)
		if err := ensurePreviewToken(&updated, &existing, c.FormValue("regen_preview") == "on"); err != nil {
			return err
		}
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
		// Move on-disk upload folder when html_upload slug changes.
		if existing.Type == model.ArticleTypeHTMLUpload && h.contentDir != "" {
			oldDir := filepath.Join(h.contentDir, existing.Slug)
			newDir := filepath.Join(h.contentDir, updated.Slug)
			_ = os.Rename(oldDir, newDir)
		}
	}
	return c.Redirect().To("/admin")
}

// Delete removes an article (and html_upload files under contentDir).
func (h *ArticleAdmin) Delete(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	// Load first so we can clean up upload files after DB delete.
	existing, err := h.repo.GetArticle(c.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	if err := h.repo.DeleteArticle(c.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	if existing.Type == model.ArticleTypeHTMLUpload && h.contentDir != "" && existing.Slug != "" {
		_ = os.RemoveAll(filepath.Join(h.contentDir, existing.Slug))
	}
	return c.Redirect().To("/admin")
}

// htmlUploadFromForm updates only metadata for html_upload articles.
// Body (entry path), Type, PreviewToken, CreatedAt are preserved.
// Password uses keep-or-hash semantics (blank keeps existing hash).
func htmlUploadFromForm(c fiber.Ctx, existing model.Article, hasher auth.PasswordHasher) model.Article {
	a := existing
	a.Slug = strings.Clone(strings.TrimSpace(c.FormValue("slug")))
	a.Title = strings.Clone(strings.TrimSpace(c.FormValue("title")))
	a.Status = model.Status(strings.Clone(c.FormValue("status")))
	a.Visibility = model.Visibility(strings.Clone(c.FormValue("visibility")))
	a.RawMode = c.FormValue("raw_mode") == "on"
	a.Pinned = c.FormValue("pinned") == "on"
	a.Tags = parseTags(c.FormValue("tags"))
	a.Password = keepOrHashPassword(c, hasher, existing.Password)
	bindSEOFields(&a, c)
	return a
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
		ShowTOC:    c.FormValue("show_toc") == "on",
	}
	bindSEOFields(&a, c)
	if raw := strings.TrimSpace(c.FormValue("publish_at")); raw != "" {
		if t, err := parseFormTime(raw); err == nil {
			a.PublishAt = &t
		}
	}
	return a
}

// bindSEOFields copies optional SEO / social form fields (cloned for fasthttp).
func bindSEOFields(a *model.Article, c fiber.Ctx) {
	a.SEOTitle = strings.Clone(strings.TrimSpace(c.FormValue("seo_title")))
	a.Summary = strings.Clone(strings.TrimSpace(c.FormValue("summary")))
	a.MetaDescription = strings.Clone(strings.TrimSpace(c.FormValue("meta_description")))
	a.CoverImageURL = strings.Clone(strings.TrimSpace(c.FormValue("cover_image_url")))
	a.OGImageURL = strings.Clone(strings.TrimSpace(c.FormValue("og_image_url")))
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
	brand := brandFromCtx(c)
	var buf bytes.Buffer
	if err := layout.Page(title, comp, seo, brand).Render(c.Context(), &buf); err != nil {
		return err
	}
	return c.Type("html").Send(buf.Bytes())
}

// brandFromCtx returns the brand loaded by middleware, or a default.
func brandFromCtx(c fiber.Ctx) sitebrand.Brand {
	if b, ok := c.Locals(localsBrandKey).(sitebrand.Brand); ok {
		return b
	}
	return sitebrand.Default("WikiBuild")
}

// BrandMiddleware loads site identity into Locals for every request.
func BrandMiddleware(repo store.Repository, fallbackTitle string) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals(localsBrandKey, sitebrand.Load(c.Context(), repo, fallbackTitle))
		return c.Next()
	}
}
