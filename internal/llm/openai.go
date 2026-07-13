package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIConfig configures an OpenAI-compatible chat completions client.
type OpenAIConfig struct {
	BaseURL    string // e.g. https://api.x.ai/v1 (no trailing slash required)
	APIKey     string
	Model      string
	HTTPClient *http.Client // optional; default has a 45s timeout
}

// OpenAIClient implements Client via POST {base}/chat/completions.
type OpenAIClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewOpenAIClient builds a client. Empty API key → Enabled() false.
func NewOpenAIClient(cfg OpenAIConfig) *OpenAIClient {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 45 * time.Second}
	}
	return &OpenAIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   strings.TrimSpace(cfg.Model),
		http:    hc,
	}
}

// Enabled is true when base URL, API key, and model are all set.
func (c *OpenAIClient) Enabled() bool {
	return c != nil && c.apiKey != "" && c.baseURL != "" && c.model != ""
}

// GenerateSEO calls chat/completions and parses a structured SEO result.
func (c *OpenAIClient) GenerateSEO(ctx context.Context, title, body string) (SEOResult, error) {
	if !c.Enabled() {
		return SEOResult{}, ErrNotConfigured
	}
	if strings.TrimSpace(body) == "" {
		return SEOResult{}, ErrEmptyBody
	}
	content, err := c.chatJSON(ctx, BuildSEOMessages(title, body))
	if err != nil {
		return SEOResult{}, err
	}
	return ParseSEOResult(content)
}

// SuggestRelated returns related catalog slugs for a selection/paragraph.
func (c *OpenAIClient) SuggestRelated(ctx context.Context, selection string, catalog []CatalogEntry) ([]RelatedSuggestion, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(selection) == "" {
		return nil, ErrEmptyBody
	}
	if len(catalog) == 0 {
		return []RelatedSuggestion{}, nil
	}
	content, err := c.chatJSON(ctx, BuildRelatedMessages(selection, catalog))
	if err != nil {
		return nil, err
	}
	return ParseRelatedResult(content)
}

func (c *OpenAIClient) chatJSON(ctx context.Context, msgs []Message) (string, error) {
	apiMsgs := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		apiMsgs = append(apiMsgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	payload := map[string]any{
		"model":    c.model,
		"messages": apiMsgs,
		"response_format": map[string]string{
			"type": "json_object",
		},
		"temperature": 0.3,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("%w: read body: %v", ErrProvider, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status %d: %s", ErrProvider, resp.StatusCode, truncate(string(respBody), 200))
	}

	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return "", fmt.Errorf("%w: envelope: %v", ErrBadResponse, err)
	}
	if len(envelope.Choices) == 0 {
		return "", fmt.Errorf("%w: no choices", ErrBadResponse)
	}
	return envelope.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
