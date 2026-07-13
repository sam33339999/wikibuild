package handler_test

import (
	"bufio"
	"context"
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
	h := handler.NewPlayground(client, "test-model")
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
	require.Contains(t, s, "LLM Playground")
	require.Contains(t, s, "WIKIBUILD_LLM_")
	require.NotContains(t, s, `id="pg-send"`)
}

func TestPlayground_Page_EnabledShowsForm(t *testing.T) {
	app := playgroundApp(t, &mockStreamClient{enabled: true})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/playground", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	require.Contains(t, s, `id="pg-send"`)
	require.Contains(t, s, `id="pg-output"`)
	require.Contains(t, s, "/static/js/playground.js")
	require.Contains(t, s, "test-model")
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
	// Fiber streaming may need longer timeout
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	// SSE data lines with deltas
	require.Contains(t, s, "Hel")
	require.Contains(t, s, "lo **world**")
	require.Contains(t, s, "[DONE]")

	require.Equal(t, 1, client.calls)
	require.GreaterOrEqual(t, len(client.lastMsg), 1)
	// system + user
	roles := make([]string, 0, len(client.lastMsg))
	for _, m := range client.lastMsg {
		roles = append(roles, m.Role)
	}
	require.Contains(t, roles, "user")
	require.Contains(t, roles, "system")
}

func TestPlayground_Stream_ReadsSSELines(t *testing.T) {
	// Ensure response is line-oriented SSE for the client parser.
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
