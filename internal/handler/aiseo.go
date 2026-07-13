package handler

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/sam33339999/wikibuild/internal/media"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/ogimage"
	"github.com/sam33339999/wikibuild/internal/store"
)

// AISEOLimit is a simple process-wide rate limit for AI SEO generation.
// Zero Max disables limiting (tests/production should set Max > 0).
type AISEOLimit struct {
	Max    int
	Window time.Duration

	mu    sync.Mutex
	times []time.Time
}

// AISEO handles admin AI SEO / related / OG helpers (does not publish articles).
type AISEO struct {
	repo       store.Repository
	client     llm.Client
	limit      *AISEOLimit
	contentDir string // html_upload files root
	mediaDir   string // generated OG images
	siteTitle  string
	now        func() time.Time
}

// NewAISEO builds a handler. client may be a disabled OpenAI client.
// limit may be nil (defaults to 10 requests / minute).
// contentDir/mediaDir may be empty (html AI / OG fail with clear errors).
func NewAISEO(repo store.Repository, client llm.Client, limit *AISEOLimit, contentDir, mediaDir, siteTitle string) *AISEO {
	if limit == nil {
		limit = &AISEOLimit{Max: 10, Window: time.Minute}
	}
	if siteTitle == "" {
		siteTitle = "WikiBuild"
	}
	return &AISEO{
		repo:       repo,
		client:     client,
		limit:      limit,
		contentDir: contentDir,
		mediaDir:   mediaDir,
		siteTitle:  siteTitle,
		now:        time.Now,
	}
}

// Generate handles POST /admin/ai/seo (title + body in form).
func (h *AISEO) Generate(c fiber.Ctx) error {
	title := strings.TrimSpace(c.FormValue("title"))
	body := c.FormValue("body")
	return h.respondSEO(c, title, body)
}

// GenerateForArticle handles POST /admin/:id/ai/seo.
// Form title/body override stored article when provided; otherwise load from DB.
// html_upload: empty body loads entry HTML from disk and strips to plain text.
func (h *AISEO) GenerateForArticle(c fiber.Ctx) error {
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
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		title = a.Title
	}
	body := c.FormValue("body")
	if strings.TrimSpace(body) == "" {
		if a.Type == model.ArticleTypeHTMLUpload {
			body, err = plainTextFromUpload(h.contentDir, a)
			if err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
		} else {
			body = a.Body
		}
	}
	return h.respondSEO(c, title, body)
}

// SuggestRelated handles POST /admin/ai/related (S3b).
// Form: selection (required), exclude_id (optional).
func (h *AISEO) SuggestRelated(c fiber.Ctx) error {
	if h.client == nil || !h.client.Enabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "llm not configured"})
	}
	selection := strings.TrimSpace(c.FormValue("selection"))
	if selection == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "selection is required"})
	}
	if !h.allow() {
		return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{"error": "rate limit exceeded; try again later"})
	}
	excludeID, _ := strconv.ParseInt(c.FormValue("exclude_id"), 10, 64)
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{Limit: 80})
	if err != nil {
		return err
	}
	catalog := make([]llm.CatalogEntry, 0, len(items))
	for _, a := range items {
		if excludeID > 0 && a.ID == excludeID {
			continue
		}
		catalog = append(catalog, llm.CatalogEntry{
			Slug:    a.Slug,
			Title:   a.Title,
			Tags:    a.Tags,
			Summary: a.Summary,
		})
	}
	sugs, err := h.client.SuggestRelated(c.Context(), selection, catalog)
	if err != nil {
		return mapLLMErr(c, err)
	}
	// Filter to known slugs only (defense in depth).
	known := make(map[string]llm.CatalogEntry, len(catalog))
	for _, e := range catalog {
		known[e.Slug] = e
	}
	out := make([]fiber.Map, 0, len(sugs))
	for _, s := range sugs {
		e, ok := known[s.Slug]
		if !ok {
			continue
		}
		title := s.Title
		if title == "" {
			title = e.Title
		}
		out = append(out, fiber.Map{
			"slug":   s.Slug,
			"title":  title,
			"reason": s.Reason,
		})
	}
	return c.JSON(fiber.Map{"suggestions": out})
}

// GenerateOG handles POST /admin/:id/ai/og — renders PNG, saves under media, returns URL.
// Does not write the article; author fills og_image_url and Saves.
func (h *AISEO) GenerateOG(c fiber.Ctx) error {
	if h.mediaDir == "" {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "media dir not configured"})
	}
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
	if !h.allow() {
		return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{"error": "rate limit exceeded; try again later"})
	}
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		title = a.Title
	}
	site := strings.TrimSpace(c.FormValue("site_title"))
	if site == "" {
		site = h.siteTitle
	}
	png, err := ogimage.Render(title, site)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	got, err := media.Save(h.mediaDir, png)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"url": got.URL, "name": got.Name})
}

func (h *AISEO) respondSEO(c fiber.Ctx, title, body string) error {
	if h.client == nil || !h.client.Enabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "llm not configured",
		})
	}
	if strings.TrimSpace(body) == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "body is required",
		})
	}
	if !h.allow() {
		return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{
			"error": "rate limit exceeded; try again later",
		})
	}

	result, err := h.client.GenerateSEO(c.Context(), title, body)
	if err != nil {
		return mapLLMErr(c, err)
	}
	return c.JSON(fiber.Map{
		"outline":          result.Outline,
		"meta_description": result.MetaDescription,
		"summary":          result.Summary,
	})
}

func (h *AISEO) allow() bool {
	if h.limit == nil || h.limit.Max <= 0 {
		return true
	}
	now := h.now()
	window := h.limit.Window
	if window <= 0 {
		window = time.Minute
	}
	h.limit.mu.Lock()
	defer h.limit.mu.Unlock()
	cutoff := now.Add(-window)
	kept := h.limit.times[:0]
	for _, t := range h.limit.times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	h.limit.times = kept
	if len(h.limit.times) >= h.limit.Max {
		return false
	}
	h.limit.times = append(h.limit.times, now)
	return true
}

func mapLLMErr(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, llm.ErrNotConfigured):
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "llm not configured"})
	case errors.Is(err, llm.ErrEmptyBody):
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "body is required"})
	case errors.Is(err, llm.ErrProvider), errors.Is(err, llm.ErrBadResponse):
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
}

// plainTextFromUpload reads html_upload entry file under contentDir/slug and strips HTML.
func plainTextFromUpload(contentDir string, a model.Article) (string, error) {
	if contentDir == "" || a.Slug == "" || a.Body == "" {
		return "", errors.New("html upload content unavailable")
	}
	// Body is entry relative path e.g. index.html — must stay inside slug dir.
	entry := filepath.Clean("/" + a.Body)
	entry = strings.TrimPrefix(entry, "/")
	if entry == "" || strings.Contains(entry, "..") {
		return "", errors.New("invalid upload entry path")
	}
	full := filepath.Join(contentDir, a.Slug, entry)
	// Ensure path is under contentDir/slug.
	root := filepath.Join(contentDir, a.Slug)
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("invalid upload path")
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", errors.New("could not read upload HTML")
	}
	text := llm.PlainTextFromHTML(string(raw))
	if strings.TrimSpace(text) == "" {
		return "", errors.New("upload HTML has no extractable text")
	}
	return text, nil
}
