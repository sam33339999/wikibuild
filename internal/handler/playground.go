package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/llm"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Playground is the admin LLM chat playground (streaming).
type Playground struct {
	client llm.Client
	model  string // display only
}

// NewPlayground builds a playground handler. client may be disabled.
func NewPlayground(client llm.Client, model string) *Playground {
	if model == "" {
		model = "(unset)"
	}
	return &Playground{client: client, model: model}
}

// Page renders GET /admin/playground.
func (h *Playground) Page(c fiber.Ctx) error {
	enabled := h.client != nil && h.client.Enabled()
	return renderPage(c, "LLM Playground", adminviews.Playground(enabled, h.model, csrf.TokenFromContext(c)))
}

type chatStreamReq struct {
	Message string `json:"message"`
	System  string `json:"system"`
}

// Stream handles POST /admin/ai/chat/stream as text/event-stream.
// Each event: data: {"delta":"..."}  then data: [DONE]
func (h *Playground) Stream(c fiber.Ctx) error {
	if h.client == nil || !h.client.Enabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "llm not configured"})
	}

	var req chatStreamReq
	if err := c.Bind().Body(&req); err != nil {
		// Fallback: form
		req.Message = c.FormValue("message")
		req.System = c.FormValue("system")
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "message is required"})
	}
	if len(msg) > llm.MaxBodyBytes {
		msg = llm.ClipBody(msg)
	}

	messages := make([]llm.Message, 0, 2)
	if sys := strings.TrimSpace(req.System); sys != "" {
		messages = append(messages, llm.Message{Role: "system", Content: sys})
	}
	messages = append(messages, llm.Message{Role: "user", Content: msg})

	c.Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Status(http.StatusOK)
	// Capture request context for the stream writer goroutine.
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
