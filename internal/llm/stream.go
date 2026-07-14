package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ParseStreamChunk extracts content delta from one JSON chunk payload (no "data:" prefix).
func ParseStreamChunk(payload string) (string, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" || payload == "[DONE]" {
		return "", nil
	}
	var env struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		return "", fmt.Errorf("%w: chunk: %v", ErrBadResponse, err)
	}
	if len(env.Choices) == 0 {
		return "", nil
	}
	return env.Choices[0].Delta.Content, nil
}

// ParseStreamDataLine parses one SSE line (with or without "data:" prefix).
// done is true for [DONE].
func ParseStreamDataLine(line string) (delta string, done bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false, nil
	}
	if strings.HasPrefix(line, "data:") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	}
	if line == "[DONE]" {
		return "", true, nil
	}
	d, err := ParseStreamChunk(line)
	return d, false, err
}

// ScanSSE reads an OpenAI-style SSE body and invokes onDelta for each content piece.
func ScanSSE(r io.Reader, onDelta func(delta string) error) error {
	sc := bufio.NewScanner(r)
	// Large chunks possible
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		delta, done, err := ParseStreamDataLine(sc.Text())
		if err != nil {
			// Skip malformed keepalive lines
			continue
		}
		if done {
			return nil
		}
		if delta == "" {
			continue
		}
		if err := onDelta(delta); err != nil {
			return err
		}
	}
	return sc.Err()
}

// StreamChat POSTs chat/completions with stream:true and forwards content deltas.
func (c *OpenAIClient) StreamChat(ctx context.Context, messages []Message, onDelta func(delta string) error) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	if len(messages) == 0 {
		return ErrEmptyBody
	}
	if onDelta == nil {
		onDelta = func(string) error { return nil }
	}

	// Cap a single stream so hung providers cannot hold the connection forever.
	ctx, cancel := context.WithTimeout(ctx, StreamHTTPTimeout)
	defer cancel()

	apiMsgs := make([]map[string]string, 0, len(messages))
	for _, m := range messages {
		apiMsgs = append(apiMsgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	payload := map[string]any{
		"model":       c.model,
		"messages":    apiMsgs,
		"stream":      true,
		"temperature": 0.7,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%w: status %d: %s", ErrProvider, resp.StatusCode, truncate(string(b), 200))
	}
	return ScanSSE(resp.Body, onDelta)
}
