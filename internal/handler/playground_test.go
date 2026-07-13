package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

type mockStreamClient struct {
	enabled bool
	deltas  []string
	err     error
	calls   int
	lastMsg []llm.Message
}

func (m *mockStreamClient) Enabled() bool { return m.enabled }

func (m *mockStreamClient) GenerateSEO(ctx context.Context, title, body string) (llm.SEOResult, error) {
	return llm.SEOResult{}, llm.ErrNotConfigured
}

func (m *mockStreamClient) SuggestRelated(ctx context.Context, selection string, catalog []llm.CatalogEntry) ([]llm.RelatedSuggestion, error) {
	return nil, llm.ErrNotConfigured
}

func (m *mockStreamClient) StreamChat(ctx context.Context, messages []llm.Message, onDelta func(string) error) error {
	m.calls++
	m.lastMsg = messages
	if m.err != nil {
		return m.err
	}
	for _, d := range m.deltas {
		if err := onDelta(d); err != nil {
			return err
		}
	}
	return nil
}

func playgroundApp(t *testing.T, client llm.Client) *fiber.App {
	t.Helper()
	h := handler.NewPlayground(client, "test-model", nil)
	app := fiber.New()
	app.Get("/admin/playground", h.Page)
	app.Post("/admin/ai/chat/stream", h.Stream)
	return app
}

func TestPlayground_Page_DisabledWithoutLLM(t *testing.T) {
	app := playgroundApp(t, &mockStreamClient{enabled: false})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/playground", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, "LLM Streaming Playground")
	require.Contains(t, s, "WIKIBUILD_LLM_")
	require.NotContains(t, s, `id="pg-send"`)
}

func TestPlayground_Page_EnabledShowsForm(t *testing.T) {
	app := playgroundApp(t, &mockStreamClient{enabled: true})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/playground", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, "LLM Streaming Playground")
	require.Contains(t, s, `id="pg-send"`)
	require.Contains(t, s, `id="pg-transcript"`)
	require.Contains(t, s, "/static/js/playground.js")
	require.Contains(t, s, "test-model")
	require.Contains(t, s, "SSE streaming")
}

func TestPlayground_Stream_NotConfigured(t *testing.T) {
	app := playgroundApp(t, &mockStreamClient{enabled: false})
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream",
		strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestPlayground_Stream_EmptyMessage(t *testing.T) {
	client := &mockStreamClient{enabled: true}
	app := playgroundApp(t, client)
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream",
		strings.NewReader(`{"message":"  "}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, 0, client.calls)
}

func TestPlayground_Stream_SSEDeltas(t *testing.T) {
	client := &mockStreamClient{
		enabled: true,
		deltas:  []string{"Hel", "lo **world**"},
	}
	app := playgroundApp(t, client)
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream",
		strings.NewReader(`{"message":"say hi","system":"be brief"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, "Hel")
	require.Contains(t, s, "lo **world**")
	require.Contains(t, s, "[DONE]")

	require.Equal(t, 1, client.calls)
	roles := make([]string, 0, len(client.lastMsg))
	for _, m := range client.lastMsg {
		roles = append(roles, m.Role)
	}
	require.Contains(t, roles, "user")
	require.Contains(t, roles, "system")
}

func TestPlayground_Stream_MultiTurnHistory(t *testing.T) {
	client := &mockStreamClient{enabled: true, deltas: []string{"ok"}}
	app := playgroundApp(t, client)
	payload := map[string]any{
		"system":  "sys",
		"message": "follow up",
		"messages": []map[string]string{
			{"role": "user", "content": "first"},
			{"role": "assistant", "content": "reply1"},
		},
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, client.calls)
	require.Len(t, client.lastMsg, 4) // system + user + assistant + follow up
	require.Equal(t, "system", client.lastMsg[0].Role)
	require.Equal(t, "first", client.lastMsg[1].Content)
	require.Equal(t, "reply1", client.lastMsg[2].Content)
	require.Equal(t, "follow up", client.lastMsg[3].Content)
}

func TestPlayground_Stream_InvalidHistoryRole(t *testing.T) {
	client := &mockStreamClient{enabled: true}
	app := playgroundApp(t, client)
	payload := `{"message":"hi","messages":[{"role":"tool","content":"x"}]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, 0, client.calls)
}

func TestPlayground_Stream_ReadsSSELines(t *testing.T) {
	client := &mockStreamClient{enabled: true, deltas: []string{"x"}}
	app := playgroundApp(t, client)
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream",
		strings.NewReader(`{"message":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	sc := bufio.NewScanner(resp.Body)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	require.NoError(t, sc.Err())
	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, "data:")
}

func TestPlayground_Stream_RateLimited(t *testing.T) {
	client := &mockStreamClient{enabled: true, deltas: []string{"x"}}
	h := handler.NewPlayground(client, "m", &handler.AISEOLimit{Max: 1, Window: time.Minute})
	app := fiber.New()
	app.Post("/admin/ai/chat/stream", h.Stream)
	post := func() *http.Response {
		req := httptest.NewRequest(http.MethodPost, "/admin/ai/chat/stream",
			strings.NewReader(`{"message":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
		require.NoError(t, err)
		return resp
	}
	require.Equal(t, http.StatusOK, post().StatusCode)
	require.Equal(t, http.StatusTooManyRequests, post().StatusCode)
	require.Equal(t, 1, client.calls)
}
