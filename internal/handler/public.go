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

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/gate"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/sam33339999/wikibuild/internal/seo"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/views/layout"
	publicviews "github.com/sam33339999/wikibuild/views/public"
)

const (
	defaultPageSize = 10
	adminCookie     = "wikibuild_admin"
	unlockTTL       = 7 * 24 * time.Hour

	// SettingDefaultProtectedPass is the settings key for the site-wide
	// default password used by protected articles without their own.
	SettingDefaultProtectedPass = "default_protected_password"
	SettingCommentProvider      = "comment_provider" // "", "giscus", "utterances"
	SettingCommentRepo          = "comment_repo"     // owner/repo
	SettingCommentCategory      = "comment_category" // giscus category name
)

// Public serves reader-facing pages. Visibility is enforced via the gate
// package: public → render, protected → password unlock (HMAC cookie bound to
// the article id), private → 404 for non-admins. The Signer verifies both the
// admin session cookie (admin bypass) and per-article unlock cookies.
type Public struct {
	repo           store.Repository
	signer         *auth.Signer
	hasher         auth.PasswordHasher
	envDefaultPass string // fallback when no DB setting is set
	contentDir     string // root for html_upload article files
	baseURL        string // no trailing slash; used for SEO canonical URLs
	pageSize       int
}

// NewPublic builds a Public handler. envDefaultPass is the fallback site-wide
// password (from config) used when no settings-managed value exists.
// contentDir is where html_upload articles' files live (<contentDir>/<slug>/).
// baseURL is optional (empty → relative SEO only).
func NewPublic(repo store.Repository, signer *auth.Signer, hasher auth.PasswordHasher, envDefaultPass, contentDir, baseURL string) *Public {
	return &Public{
		repo:           repo,
		signer:         signer,
		hasher:         hasher,
		envDefaultPass: envDefaultPass,
		contentDir:     contentDir,
		baseURL:        strings.TrimRight(baseURL, "/"),
		pageSize:       defaultPageSize,
	}
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

// Article renders a single article by slug, applying the visibility gate.
// Unknown slugs fall through to the redirects table (301).
func (h *Public) Article(c fiber.Ctx) error {
	slug := c.Params("slug")
	a, err := h.repo.GetArticleBySlug(c.Context(), slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			ok, rerr := tryRedirect(c, h.repo, "/"+slug)
			if rerr != nil {
				return rerr
			}
			if ok {
				return nil
			}
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}

	decision := gate.Decide(gate.AccessInput{
		Status:     a.Status,
		Visibility: a.Visibility,
		IsAdmin:    h.isAdmin(c),
		Unlocked:   h.isUnlocked(c, a.ID),
	})
	switch decision {
	case gate.Allow:
		return h.renderArticle(c, a)
	case gate.Password:
		return c.Redirect().Status(http.StatusFound).To("/" + a.Slug + "/unlock")
	default: // gate.NotFound
		return c.SendStatus(http.StatusNotFound)
	}
}

// UnlockForm renders the password page for a protected article.
func (h *Public) UnlockForm(c fiber.Ctx) error {
	a, ok := h.protectedArticle(c)
	if !ok {
		return c.SendStatus(http.StatusNotFound)
	}
	if h.isAdmin(c) || h.isUnlocked(c, a.ID) {
		return c.Redirect().To("/" + a.Slug)
	}
	return renderPage(c, "解鎖："+a.Title, publicviews.Unlock(a.Slug, csrf.TokenFromContext(c), ""))
}

// UnlockSubmit verifies the password and sets a signed unlock cookie bound to
// the article id. Wrong passwords re-render the form with an error.
func (h *Public) UnlockSubmit(c fiber.Ctx) error {
	a, ok := h.protectedArticle(c)
	if !ok {
		return c.SendStatus(http.StatusNotFound)
	}
	if !gate.MatchPassword(a, c.FormValue("password"), h.siteDefault(c), h.hasher) {
		return renderPage(c, "解鎖："+a.Title,
			publicviews.Unlock(a.Slug, csrf.TokenFromContext(c), "密碼不正確"))
	}
	tok, err := h.signer.Sign(strconv.FormatInt(a.ID, 10), unlockTTL)
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name:     unlockCookieName(a.ID),
		Value:    tok,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   int(unlockTTL.Seconds()),
	})
	return c.Redirect().To("/" + a.Slug)
}

// renderArticle serves an allowed article. Markdown is rendered to HTML with a
// TOC; html_upload is either raw (full page) or framed in an iframe under the
// site chrome so relative paths and the upload's own CSS still work.
func (h *Public) renderArticle(c fiber.Ctx, a model.Article) error {
	comments := h.commentConfig(c)
	desc := feedSummary(a.Body)
	pageSEO := layout.SEO{
		Type:        "article",
		Description: desc,
		JSONLD:      seo.ArticleJSONLD(h.baseURL, a, desc),
	}
	if h.baseURL != "" {
		pageSEO.Canonical = h.baseURL + "/" + a.Slug
	}
	if a.Type != model.ArticleTypeHTMLUpload {
		html, toc := render.RenderWithTOC(a.Body)
		return renderPageSEO(c, a.Title, publicviews.Article(
			a, html, toc, render.ReadingTime(a.Body), h.backlinksFor(c, a), comments), pageSEO)
	}

	// raw_mode: original document only (no site header). Relative assets still
	// need <base href="/slug/"> + GET /:slug/*.
	if a.RawMode {
		return h.serveUploadDocument(c, a)
	}
	// Keep site chrome; show the full upload inside an iframe pointed at
	// /:slug/~content so slides/css resolve inside the frame, not the outer page.
	// Do NOT set layout BaseHref — that would break site nav links.
	src := "/" + a.Slug + "/~content"
	return renderPageSEO(c, a.Title, publicviews.HTMLFrame(a, src), pageSEO)
}

// UploadContent serves the raw html_upload document (with <base>) for iframe
// embedding. Path: GET /:slug/~content
func (h *Public) UploadContent(c fiber.Ctx) error {
	a, err := h.repo.GetArticleBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return c.SendStatus(http.StatusNotFound)
	}
	if a.Type != model.ArticleTypeHTMLUpload {
		return c.SendStatus(http.StatusNotFound)
	}
	decision := gate.Decide(gate.AccessInput{
		Status:     a.Status,
		Visibility: a.Visibility,
		IsAdmin:    h.isAdmin(c),
		Unlocked:   h.isUnlocked(c, a.ID),
	})
	if decision != gate.Allow {
		return c.SendStatus(http.StatusNotFound)
	}
	return h.serveUploadDocument(c, a)
}

// serveUploadDocument reads the entry HTML and returns it with a <base> tag so
// relative URLs resolve under /:slug/.
func (h *Public) serveUploadDocument(c fiber.Ctx, a model.Article) error {
	data, err := readUploadFile(h.contentDir, a.Slug, a.Body)
	if err != nil {
		if os.IsNotExist(err) {
			return c.Status(http.StatusNotFound).SendString(
				"upload entry file missing (re-upload the zip after the nested-folder fix)")
		}
		return err
	}
	base := "/" + a.Slug + "/"
	return c.Type("html").Send(ensureBaseHref(data, base))
}

// UploadAsset serves a file under contentDir/<slug>/ for html_upload articles.
// Registered as GET /:slug/* so relative links from the entry page work.
// Only published public (or admin) articles expose assets; private/protected
// without access return 404.
func (h *Public) UploadAsset(c fiber.Ctx) error {
	slug := c.Params("slug")
	// Fiber wildcard may be named "*" or include leading slash depending on version.
	rel := c.Params("*")
	if rel == "" {
		rel = strings.TrimPrefix(c.Path(), "/"+slug+"/")
	}
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return c.SendStatus(http.StatusNotFound)
	}

	a, err := h.repo.GetArticleBySlug(c.Context(), slug)
	if err != nil {
		return c.SendStatus(http.StatusNotFound)
	}
	if a.Type != model.ArticleTypeHTMLUpload {
		return c.SendStatus(http.StatusNotFound)
	}
	// Same visibility gate as the article page (simplified: public+published
	// or admin; protected needs unlock cookie).
	decision := gate.Decide(gate.AccessInput{
		Status:     a.Status,
		Visibility: a.Visibility,
		IsAdmin:    h.isAdmin(c),
		Unlocked:   h.isUnlocked(c, a.ID),
	})
	if decision != gate.Allow {
		return c.SendStatus(http.StatusNotFound)
	}

	path := filepath.Join(h.contentDir, slug, filepath.FromSlash(rel))
	if !within(filepath.Join(h.contentDir, slug), path) {
		return c.SendStatus(http.StatusNotFound)
	}
	return c.SendFile(path)
}

// ensureBaseHref inserts <base href="..."> after <head> when the document has
// no base tag yet, so relative asset URLs resolve under the article slug.
func ensureBaseHref(html []byte, href string) []byte {
	lower := bytes.ToLower(html)
	if bytes.Contains(lower, []byte("<base")) {
		return html
	}
	tag := []byte(`<base href="` + href + `">`)
	// Prefer after <head ...>
	if i := bytes.Index(lower, []byte("<head")); i >= 0 {
		// find end of opening head tag
		j := bytes.IndexByte(html[i:], '>')
		if j >= 0 {
			at := i + j + 1
			out := make([]byte, 0, len(html)+len(tag)+1)
			out = append(out, html[:at]...)
			out = append(out, '\n')
			out = append(out, tag...)
			out = append(out, html[at:]...)
			return out
		}
	}
	// Fallback: prepend
	return append(tag, html...)
}

func (h *Public) commentConfig(c fiber.Ctx) publicviews.CommentConfig {
	provider, _ := h.repo.GetSetting(c.Context(), SettingCommentProvider)
	repo, _ := h.repo.GetSetting(c.Context(), SettingCommentRepo)
	cat, _ := h.repo.GetSetting(c.Context(), SettingCommentCategory)
	return publicviews.CommentConfig{
		Provider: provider,
		Repo:     repo,
		Category: cat,
	}
}

// feedSummary reuses a short plain-text blurb for meta description.
func feedSummary(body string) string {
	s := strings.TrimSpace(body)
	s = strings.ReplaceAll(s, "#", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// backlinksFor returns published, public articles whose body wikilinks to a.
// Markdown bodies may contain [[a.Slug]]; we search for that token and exclude
// the article itself (self-links aren't backlinks).
func (h *Public) backlinksFor(c fiber.Ctx, a model.Article) []model.Article {
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
		Search:     "[[" + a.Slug + "]]",
	})
	if err != nil {
		return nil
	}
	out := items[:0]
	for _, b := range items {
		if b.ID != a.ID {
			out = append(out, b)
		}
	}
	return out
}

// readUploadFile reads <contentDir>/<slug>/<name> safely, refusing to escape
// the slug directory.
func readUploadFile(contentDir, slug, name string) ([]byte, error) {
	path := filepath.Join(contentDir, slug, name)
	if !within(filepath.Join(contentDir, slug), path) {
		return nil, errors.New("upload file path escapes slug directory")
	}
	return os.ReadFile(path)
}
func (h *Public) protectedArticle(c fiber.Ctx) (model.Article, bool) {
	a, err := h.repo.GetArticleBySlug(c.Context(), c.Params("slug"))
	if err != nil || a.Status != model.StatusPublished || a.Visibility != model.VisibilityProtected {
		return model.Article{}, false
	}
	return a, true
}

// siteDefault resolves the site-wide protected password: the settings-managed
// value if set, otherwise the env fallback. Read per-unlock so settings
// changes take effect immediately.
func (h *Public) siteDefault(c fiber.Ctx) string {
	if v, _ := h.repo.GetSetting(c.Context(), SettingDefaultProtectedPass); v != "" {
		return v
	}
	return h.envDefaultPass
}

// isAdmin reports whether the request carries a valid admin session cookie.
func (h *Public) isAdmin(c fiber.Ctx) bool {
	tok := c.Cookies(adminCookie)
	if tok == "" {
		return false
	}
	_, err := h.signer.Verify(tok)
	return err == nil
}

// isUnlocked reports whether the request carries a valid, article-bound unlock
// cookie for the given article id.
func (h *Public) isUnlocked(c fiber.Ctx, articleID int64) bool {
	tok := c.Cookies(unlockCookieName(articleID))
	if tok == "" {
		return false
	}
	payload, err := h.signer.Verify(tok)
	return err == nil && payload == strconv.FormatInt(articleID, 10)
}

func unlockCookieName(articleID int64) string {
	return "wikibuild_unlock_" + strconv.FormatInt(articleID, 10)
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
