// Package llm provides an OpenAI-compatible client for admin AI features.
// Unit tests use httptest or interface mocks — never a live provider.
package llm

import (
	"context"
	"errors"
)

// MaxBodyBytes is the maximum article body size sent to the provider.
// Longer bodies are clipped (UTF-8 safe) before the request.
const MaxBodyBytes = 64 * 1024

// Sentinel errors for callers (handlers map these to HTTP status).
var (
	ErrNotConfigured = errors.New("llm: not configured")
	ErrEmptyBody     = errors.New("llm: empty body")
	ErrBadResponse   = errors.New("llm: bad response")
	ErrProvider      = errors.New("llm: provider error")
)

// SEOResult is the structured output of GenerateSEO (pre-fill form fields).
type SEOResult struct {
	Outline         []string
	MetaDescription string
	Summary         string
}

// Message is one chat message for OpenAI-compatible APIs.
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // assistant
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool
	Name       string     `json:"name,omitempty"`         // tool function name (optional)
}

// Client generates SEO fields, related suggestions, and optional chat streams.
// Enabled reports whether the client has credentials to make requests.
type Client interface {
	Enabled() bool
	GenerateSEO(ctx context.Context, title, body string) (SEOResult, error)
	SuggestRelated(ctx context.Context, selection string, catalog []CatalogEntry) ([]RelatedSuggestion, error)
	// StreamChat streams assistant text deltas (OpenAI stream:true). onDelta may be called many times.
	StreamChat(ctx context.Context, messages []Message, onDelta func(delta string) error) error
	// Chat is a non-stream completion, optionally with tools (for agent loops).
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (ChatResult, error)
}
