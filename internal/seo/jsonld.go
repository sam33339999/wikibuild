// Package seo builds structured data snippets (JSON-LD) for public pages.
// Pure helpers — no I/O.
package seo

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/sam33339999/wikibuild/internal/model"
)

// ArticleJSONLD returns a schema.org BlogPosting JSON-LD object as a string.
// baseURL has no trailing slash; empty baseURL yields an empty string (skip).
// headline, description, and image should be effective (fallback-resolved) values.
func ArticleJSONLD(baseURL string, a model.Article, description, image string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" || a.Slug == "" {
		return ""
	}
	type blogPosting struct {
		Context       string `json:"@context"`
		Type          string `json:"@type"`
		Headline      string `json:"headline"`
		Description   string `json:"description,omitempty"`
		URL           string `json:"url"`
		Image         string `json:"image,omitempty"`
		DatePublished string `json:"datePublished,omitempty"`
		DateModified  string `json:"dateModified,omitempty"`
	}
	headline := EffectiveTitle(a)
	bp := blogPosting{
		Context:     "https://schema.org",
		Type:        "BlogPosting",
		Headline:    headline,
		Description: description,
		URL:         baseURL + "/" + a.Slug,
		Image:       strings.TrimSpace(image),
	}
	if a.PublishedAt != nil {
		bp.DatePublished = a.PublishedAt.UTC().Format(time.RFC3339)
	}
	if !a.UpdatedAt.IsZero() {
		bp.DateModified = a.UpdatedAt.UTC().Format(time.RFC3339)
	}
	raw, err := json.Marshal(bp)
	if err != nil {
		return ""
	}
	return string(raw)
}
