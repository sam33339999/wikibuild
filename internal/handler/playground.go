package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/sam33339999/wikibuild/internal/mcp"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// PlaygroundAgentTimeout is the max wall time for one playground stream
// (multi-round tool use can take several minutes).
const PlaygroundAgentTimeout = 6 * time.Minute

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
// Sends SSE comments as keepalive while the model is thinking so browsers/proxies
// do not close an idle long-running stream.
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

	// Bound total agent time; still cancel if the client disconnects (parent ctx).
	reqCtx := c.Context()
	agentCtx, cancel := context.WithTimeout(reqCtx, PlaygroundAgentTimeout)
	// cancel is called when the stream writer finishes (below).

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer cancel()

		var mu sync.Mutex
		writeRaw := func(s string) error {
			mu.Lock()
			defer mu.Unlock()
			if _, err := fmt.Fprint(w, s); err != nil {
				return err
			}
			return w.Flush()
		}
		writeSSE := func(payload string) error {
			return writeRaw(fmt.Sprintf("data: %s\n\n", payload))
		}
		writeJSON := func(v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			return writeSSE(string(b))
		}

		// Keepalive comments every 8s while the model is thinking (no data frames).
		// Prevents idle proxies/browsers from closing the stream mid tool-round.
		stopKA := make(chan struct{})
		var kaOnce sync.Once
		stopKeepalive := func() { kaOnce.Do(func() { close(stopKA) }) }
		go func() {
			t := time.NewTicker(8 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-stopKA:
					return
				case <-agentCtx.Done():
					return
				case <-t.C:
					_ = writeRaw(": keepalive\n\n")
				}
			}
		}()
		defer stopKeepalive()

		// Immediate status so the client sees activity right away.
		_ = writeJSON(map[string]any{"type": "status", "message": "connected"})

		var runErr error
		if useTools {
			runErr = llm.RunAgent(agentCtx, h.client, h.tools, messages, llm.ArticleToolDefs(), func(ev llm.AgentEvent) error {
				switch ev.Type {
				case "delta":
					return writeJSON(map[string]string{"delta": ev.Delta})
				case "status":
					return writeJSON(map[string]any{"type": "status", "message": ev.Message})
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
			_ = writeJSON(map[string]any{"type": "status", "message": "streaming…"})
			runErr = h.client.StreamChat(agentCtx, messages, func(delta string) error {
				if delta == "" {
					return nil
				}
				return writeJSON(map[string]string{"delta": delta})
			})
		}
		stopKeepalive()
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
