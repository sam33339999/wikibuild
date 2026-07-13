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
	Role    string
	Content string
}

// Client generates SEO fields and related-article suggestions.
// Enabled reports whether the client has credentials to make requests.
type Client interface {
	Enabled() bool
	GenerateSEO(ctx context.Context, title, body string) (SEOResult, error)
	SuggestRelated(ctx context.Context, selection string, catalog []CatalogEntry) ([]RelatedSuggestion, error)
}
