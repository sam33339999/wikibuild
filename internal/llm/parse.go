package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

type seoJSON struct {
	Outline         []string `json:"outline"`
	MetaDescription string   `json:"meta_description"`
	Summary         string   `json:"summary"`
}

// ParseSEOResult extracts SEOResult from model content (raw JSON, fenced, or embedded).
func ParseSEOResult(content string) (SEOResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return SEOResult{}, fmt.Errorf("%w: empty content", ErrBadResponse)
	}

	// Prefer fenced ```json ... ``` then first {...} blob.
	if s, ok := extractFencedJSON(content); ok {
		content = s
	} else if s, ok := extractFirstObject(content); ok {
		content = s
	}

	var raw seoJSON
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return SEOResult{}, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	meta := strings.TrimSpace(raw.MetaDescription)
	sum := strings.TrimSpace(raw.Summary)
	if meta == "" || sum == "" {
		return SEOResult{}, fmt.Errorf("%w: meta_description and summary are required", ErrBadResponse)
	}
	outline := make([]string, 0, len(raw.Outline))
	for _, o := range raw.Outline {
		if t := strings.TrimSpace(o); t != "" {
			outline = append(outline, t)
		}
	}
	return SEOResult{
		Outline:         outline,
		MetaDescription: meta,
		Summary:         sum,
	}, nil
}

func extractFencedJSON(s string) (string, bool) {
	const open = "```"
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	// optional language tag
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	}
	j := strings.Index(rest, open)
	if j < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:j]), true
}

func extractFirstObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}
