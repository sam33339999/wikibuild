package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/sam33339999/wikibuild/internal/mcp"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Playground is the admin LLM streaming chat playground (optional article tools).
type Playground struct {
	client llm.Client
	model  string // display only
	limit  *AISEOLimit
	tools  *mcp.Tools // nil → tools mode unavailable
}

// NewPlayground builds a playground handler. client may be disabled.
// tools may be nil (tool use disabled server-side).
// limit may be nil (defaults to 30 streams / minute).
func NewPlayground(client llm.Client, model string, tools *mcp.Tools, limit *AISEOLimit) *Playground {
	if model == "" {
		model = "(unset)"
	}
	if limit == nil {
		limit = &AISEOLimit{Max: 30, Window: time.Minute}
	}
	return &Playground{client: client, model: model, tools: tools, limit: limit}
}

// Page renders GET /admin/playground.
func (h *Playground) Page(c fiber.Ctx) error {
	enabled := h.client != nil && h.client.Enabled()
	toolsOK := h.tools != nil
	return renderPage(c, "LLM Streaming Playground", adminviews.Playground(enabled, toolsOK, h.model, csrf.TokenFromContext(c)))
}

type chatStreamReq struct {
	Message  string        `json:"message"`
	System   string        `json:"system"`
	Messages []llm.Message `json:"messages"`
	// Tools enables MCP article tools (list/get/create/update/status/visibility).
	Tools bool `json:"tools"`
}

// Stream handles POST /admin/ai/chat/stream as text/event-stream.
// Events: {"delta":...} | {"type":"tool_call",...} | {"type":"tool_result",...} | [DONE]
func (h *Playground) Stream(c fiber.Ctx) error {
	if h.client == nil || !h.client.Enabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "llm not configured"})
	}

	var req chatStreamReq
	if err := c.Bind().Body(&req); err != nil {
		req.Message = c.FormValue("message")
		req.System = c.FormValue("system")
		req.Tools = c.FormValue("tools") == "1" || c.FormValue("tools") == "true"
	}

	system := req.System
	if req.Tools && h.tools != nil {
		// Nudge the model to use tools when enabled.
		hint := "You can use article tools to list/get/create/update WikiBuild posts. Prefer tools for site data. Defaults for create are draft+private."
		if strings.TrimSpace(system) == "" {
			system = hint
		} else {
			system = system + "\n\n" + hint
		}
	}

	messages, err := llm.BuildChatMessages(system, req.Messages, req.Message)
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

	useTools := req.Tools && h.tools != nil

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
		writeJSON := func(v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			return writeSSE(string(b))
		}

		var runErr error
		if useTools {
			runErr = llm.RunAgent(reqCtx, h.client, h.tools, messages, llm.ArticleToolDefs(), func(ev llm.AgentEvent) error {
				switch ev.Type {
				case "delta":
					return writeJSON(map[string]string{"delta": ev.Delta})
				case "tool_call":
					return writeJSON(map[string]any{
						"type": "tool_call", "id": ev.ID, "name": ev.Name, "arguments": ev.Args,
					})
				case "tool_result":
					return writeJSON(map[string]any{
						"type": "tool_result", "id": ev.ID, "name": ev.Name, "result": ev.Result,
					})
				default:
					return nil
				}
			})
		} else {
			runErr = h.client.StreamChat(reqCtx, messages, func(delta string) error {
				if delta == "" {
					return nil
				}
				return writeJSON(map[string]string{"delta": delta})
			})
		}
		if runErr != nil {
			_ = writeJSON(map[string]string{"error": runErr.Error()})
		}
		_ = writeSSE("[DONE]")
	})
}

func (h *Playground) allow() bool {
	if h.limit == nil || h.limit.Max <= 0 {
		return true
	}
	tmp := &AISEO{limit: h.limit, now: time.Now}
	return tmp.allow()
}

