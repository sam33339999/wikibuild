package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/gate"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/sam33339999/wikibuild/internal/store"
	publicviews "github.com/sam33339999/wikibuild/views/public"
)

const (
	defaultPageSize = 10
	adminCookie     = "wikibuild_admin"
	unlockTTL       = 7 * 24 * time.Hour
)

// Public serves reader-facing pages. Visibility is enforced via the gate
// package: public → render, protected → password unlock (HMAC cookie bound to
// the article id), private → 404 for non-admins. The Signer verifies both the
// admin session cookie (admin bypass) and per-article unlock cookies.
type Public struct {
	repo            store.Repository
	signer          *auth.Signer
	hasher          auth.PasswordHasher
	siteDefaultPass string
	pageSize        int
}

// NewPublic builds a Public handler. siteDefaultPass is the fallback password
// for protected articles without their own (M2.3 adds a settings-managed one).
func NewPublic(repo store.Repository, signer *auth.Signer, hasher auth.PasswordHasher, siteDefaultPass string) *Public {
	return &Public{
		repo:            repo,
		signer:          signer,
		hasher:          hasher,
		siteDefaultPass: siteDefaultPass,
		pageSize:        defaultPageSize,
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
func (h *Public) Article(c fiber.Ctx) error {
	a, err := h.repo.GetArticleBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
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
		html, toc := render.RenderWithTOC(a.Body)
		return renderPage(c, a.Title, publicviews.Article(a, html, toc))
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
	if !gate.MatchPassword(a, c.FormValue("password"), h.siteDefaultPass, h.hasher) {
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

// protectedArticle fetches the article by :slug and returns it only if it is a
// published, protected article (otherwise the unlock page must not exist).
func (h *Public) protectedArticle(c fiber.Ctx) (model.Article, bool) {
	a, err := h.repo.GetArticleBySlug(c.Context(), c.Params("slug"))
	if err != nil || a.Status != model.StatusPublished || a.Visibility != model.VisibilityProtected {
		return model.Article{}, false
	}
	return a, true
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
