package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/sam33339999/wikibuild/internal/store"
)

// AISEOLimit is a simple process-wide rate limit for AI SEO generation.
// Zero Max disables limiting (tests/production should set Max > 0).
type AISEOLimit struct {
	Max    int
	Window time.Duration

	mu     sync.Mutex
	times  []time.Time
}

// AISEO handles admin AI SEO generation (does not persist articles).
type AISEO struct {
	repo   store.Repository
	client llm.Client
	limit  *AISEOLimit
	now    func() time.Time
}

// NewAISEO builds a handler. client may be a disabled OpenAI client.
// limit may be nil (defaults to 10 requests / minute).
func NewAISEO(repo store.Repository, client llm.Client, limit *AISEOLimit) *AISEO {
	if limit == nil {
		limit = &AISEOLimit{Max: 10, Window: time.Minute}
	}
	return &AISEO{
		repo:   repo,
		client: client,
		limit:  limit,
		now:    time.Now,
	}
}

// Generate handles POST /admin/ai/seo (title + body in form).
func (h *AISEO) Generate(c fiber.Ctx) error {
	title := strings.TrimSpace(c.FormValue("title"))
	body := c.FormValue("body")
	return h.respond(c, title, body)
}

// GenerateForArticle handles POST /admin/:id/ai/seo.
// Form title/body override stored article when provided; otherwise load from DB.
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
		body = a.Body
	}
	return h.respond(c, title, body)
}

func (h *AISEO) respond(c fiber.Ctx, title, body string) error {
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
	// Drop expired.
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
