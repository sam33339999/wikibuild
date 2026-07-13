package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/llm"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Playground is the admin LLM streaming chat playground.
type Playground struct {
	client llm.Client
	model  string // display only
	limit  *AISEOLimit
}

// NewPlayground builds a playground handler. client may be disabled.
// limit may be nil (defaults to 30 streams / minute).
func NewPlayground(client llm.Client, model string, limit *AISEOLimit) *Playground {
	if model == "" {
		model = "(unset)"
	}
	if limit == nil {
		limit = &AISEOLimit{Max: 30, Window: time.Minute}
	}
	return &Playground{client: client, model: model, limit: limit}
}

// Page renders GET /admin/playground.
func (h *Playground) Page(c fiber.Ctx) error {
	enabled := h.client != nil && h.client.Enabled()
	return renderPage(c, "LLM Streaming Playground", adminviews.Playground(enabled, h.model, csrf.TokenFromContext(c)))
}

type chatStreamReq struct {
	Message  string        `json:"message"`
	System   string        `json:"system"`
	Messages []llm.Message `json:"messages"` // prior turns (user/assistant only)
}

// Stream handles POST /admin/ai/chat/stream as text/event-stream.
// Each event: data: {"delta":"..."}  then data: [DONE]
// Supports multi-turn via messages[] history + latest message.
func (h *Playground) Stream(c fiber.Ctx) error {
	if h.client == nil || !h.client.Enabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "llm not configured"})
	}

	var req chatStreamReq
	if err := c.Bind().Body(&req); err != nil {
		req.Message = c.FormValue("message")
		req.System = c.FormValue("system")
	}

	messages, err := llm.BuildChatMessages(req.System, req.Messages, req.Message)
	if err != nil {
		if errors.Is(err, llm.ErrEmptyBody) {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "message is required"})
		}
		if errors.Is(err, llm.ErrInvalidChat) {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid chat history"})
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if !h.allow() {
		return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{
			"error": "rate limit exceeded; try again later",
		})
	}

	c.Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)
	reqCtx := c.Context()
	return c.SendStreamWriter(func(w *bufio.Writer) {
		writeSSE := func(payload string) error {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return err
			}
			return w.Flush()
		}
		err := h.client.StreamChat(reqCtx, messages, func(delta string) error {
			if delta == "" {
				return nil
			}
			b, err := json.Marshal(map[string]string{"delta": delta})
			if err != nil {
				return err
			}
			return writeSSE(string(b))
		})
		if err != nil {
			b, _ := json.Marshal(map[string]string{"error": err.Error()})
			_ = writeSSE(string(b))
		}
		_ = writeSSE("[DONE]")
	})
}

func (h *Playground) allow() bool {
	if h.limit == nil || h.limit.Max <= 0 {
		return true
	}
	// Reuse AISEOLimit mutex/window semantics.
	tmp := &AISEO{limit: h.limit, now: time.Now}
	return tmp.allow()
}
