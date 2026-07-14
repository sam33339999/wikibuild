package llm_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestParseStreamChunk_ContentDelta(t *testing.T) {
	// OpenAI-style streaming chunk
	raw := `{"choices":[{"delta":{"content":"Hello"},"index":0}]}`
	got, err := llm.ParseStreamChunk(raw)
	require.NoError(t, err)
	require.Equal(t, "Hello", got)
}

func TestParseStreamChunk_EmptyDelta(t *testing.T) {
	raw := `{"choices":[{"delta":{},"index":0}]}`
	got, err := llm.ParseStreamChunk(raw)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestParseStreamChunk_Done(t *testing.T) {
	got, done, err := llm.ParseStreamDataLine("[DONE]")
	require.NoError(t, err)
	require.True(t, done)
	require.Empty(t, got)
}

func TestParseStreamDataLine_DataPrefix(t *testing.T) {
	got, done, err := llm.ParseStreamDataLine(`data: {"choices":[{"delta":{"content":"x"}}]}`)
	require.NoError(t, err)
	require.False(t, done)
	require.Equal(t, "x", got)
}

func TestScanSSE_AccumulatesDeltas(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out strings.Builder
	err := llm.ScanSSE(strings.NewReader(body), func(delta string) error {
		out.WriteString(delta)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "Hello", out.String())
}

func TestScanSSE_IgnoresKeepaliveComments(t *testing.T) {
	body := strings.Join([]string{
		`: keepalive`,
		``,
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		``,
		`: still-here`,
		`data: [DONE]`,
		``,
	}, "\n")
	var out strings.Builder
	err := llm.ScanSSE(strings.NewReader(body), func(delta string) error {
		out.WriteString(delta)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", out.String())
}

func TestOpenAIClient_StreamChat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		require.Contains(t, string(raw), `"stream":true`)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)

	c := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL:    srv.URL + "/v1",
		APIKey:     "k",
		Model:      "m",
		HTTPClient: srv.Client(),
	})
	var got strings.Builder
	err := c.StreamChat(context.Background(), []llm.Message{
		{Role: "user", Content: "hi"},
	}, func(delta string) error {
		got.WriteString(delta)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "AB", got.String())
}

func TestOpenAIClient_StreamChat_NotConfigured(t *testing.T) {
	c := llm.NewOpenAIClient(llm.OpenAIConfig{})
	err := c.StreamChat(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, func(string) error { return nil })
	require.ErrorIs(t, err, llm.ErrNotConfigured)
}

func TestOpenAIClient_StreamChat_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		time.Sleep(300 * time.Millisecond)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
	}))
	t.Cleanup(srv.Close)

	c := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL:    srv.URL,
		APIKey:     "k",
		Model:      "m",
		HTTPClient: srv.Client(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.StreamChat(ctx, []llm.Message{{Role: "user", Content: "x"}}, func(string) error { return nil })
	require.Error(t, err)
}
