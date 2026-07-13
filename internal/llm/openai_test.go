package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClient_NotConfigured(t *testing.T) {
	c := llm.NewOpenAIClient(llm.OpenAIConfig{})
	require.False(t, c.Enabled())
	_, err := c.GenerateSEO(context.Background(), "T", "body")
	require.ErrorIs(t, err, llm.ErrNotConfigured)
}

func TestOpenAIClient_EmptyBody(t *testing.T) {
	c := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL: "http://example.invalid/v1",
		APIKey:  "k",
		Model:   "m",
	})
	require.True(t, c.Enabled())
	_, err := c.GenerateSEO(context.Background(), "T", "  \n")
	require.ErrorIs(t, err, llm.ErrEmptyBody)
}

func TestOpenAIClient_GenerateSEO_Success(t *testing.T) {
	var gotAuth, gotPath, gotModel string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		if m, ok := gotBody["model"].(string); ok {
			gotModel = m
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{
					"role":"assistant",
					"content":"{\"outline\":[\"One\"],\"meta_description\":\"Meta text.\",\"summary\":\"Summary text.\"}"
				}
			}]
		}`))
	}))
	t.Cleanup(srv.Close)

	c := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL:    srv.URL + "/v1",
		APIKey:     "secret-key",
		Model:      "test-model",
		HTTPClient: srv.Client(),
	})
	got, err := c.GenerateSEO(context.Background(), "Title", "Body paragraph.")
	require.NoError(t, err)
	require.Equal(t, []string{"One"}, got.Outline)
	require.Equal(t, "Meta text.", got.MetaDescription)
	require.Equal(t, "Summary text.", got.Summary)

	require.Equal(t, "Bearer secret-key", gotAuth)
	require.True(t, strings.HasSuffix(gotPath, "/chat/completions"), "path=%s", gotPath)
	require.Equal(t, "test-model", gotModel)
	// Prefer structured JSON when provider supports it.
	rf, _ := gotBody["response_format"].(map[string]any)
	require.Equal(t, "json_object", rf["type"])
}

func TestOpenAIClient_GenerateSEO_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	c := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL:    srv.URL,
		APIKey:     "k",
		Model:      "m",
		HTTPClient: srv.Client(),
	})
	_, err := c.GenerateSEO(context.Background(), "T", "body")
	require.Error(t, err)
	require.ErrorIs(t, err, llm.ErrProvider)
}

func TestOpenAIClient_GenerateSEO_RespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
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
	_, err := c.GenerateSEO(ctx, "T", "body")
	require.Error(t, err)
}
