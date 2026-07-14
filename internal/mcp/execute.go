package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Execute runs a named article tool with a JSON arguments object (OpenAI tool call style).
// Returns a JSON string result suitable for role=tool messages.
func (t *Tools) Execute(ctx context.Context, name, argumentsJSON string) (string, error) {
	name = strings.TrimSpace(name)
	args := strings.TrimSpace(argumentsJSON)
	if args == "" {
		args = "{}"
	}
	switch name {
	case "list_articles":
		var in ListInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		items, err := t.ListArticles(ctx, in)
		return marshalTool(items, err)
	case "get_article":
		var in GetInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		// Accept float ids from JSON numbers via generic map fallback
		in = coerceGetInput(args, in)
		item, err := t.GetArticle(ctx, in)
		return marshalTool(item, err)
	case "create_article":
		var in CreateInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		item, err := t.CreateArticle(ctx, in)
		return marshalTool(item, err)
	case "update_article":
		in, err := parseUpdateInput(args)
		if err != nil {
			return "", err
		}
		item, err := t.UpdateArticle(ctx, in)
		return marshalTool(item, err)
	case "set_article_status":
		var in SetStatusInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		in.ID = coerceID(args, "id", in.ID)
		item, err := t.SetArticleStatus(ctx, in)
		return marshalTool(item, err)
	case "set_article_visibility":
		var in SetVisibilityInput
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		in.ID = coerceID(args, "id", in.ID)
		item, err := t.SetArticleVisibility(ctx, in)
		return marshalTool(item, err)
	default:
		return "", fmt.Errorf("%w: unknown tool %q", ErrInvalidInput, name)
	}
}

func marshalTool(v any, err error) (string, error) {
	if err != nil {
		// Still return JSON error payload so the model can read it.
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(b), nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func coerceGetInput(args string, in GetInput) GetInput {
	if in.ID > 0 || strings.TrimSpace(in.Slug) != "" {
		return in
	}
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return in
	}
	if id, ok := asInt64(m["id"]); ok {
		in.ID = id
	}
	if s, ok := m["slug"].(string); ok {
		in.Slug = s
	}
	return in
}

func coerceID(args, key string, cur int64) int64 {
	if cur > 0 {
		return cur
	}
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return cur
	}
	if id, ok := asInt64(m[key]); ok {
		return id
	}
	return cur
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func parseUpdateInput(args string) (UpdateInput, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return UpdateInput{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	var in UpdateInput
	if id, ok := asInt64(raw["id"]); ok {
		in.ID = id
	}
	strPtr := func(k string) *string {
		if v, ok := raw[k]; ok {
			if s, ok := v.(string); ok {
				return &s
			}
		}
		return nil
	}
	boolPtr := func(k string) *bool {
		if v, ok := raw[k]; ok {
			if b, ok := v.(bool); ok {
				return &b
			}
		}
		return nil
	}
	in.Slug = strPtr("slug")
	in.Title = strPtr("title")
	in.Body = strPtr("body")
	in.SEOTitle = strPtr("seo_title")
	in.Summary = strPtr("summary")
	in.MetaDescription = strPtr("meta_description")
	in.CoverImageURL = strPtr("cover_image_url")
	in.OGImageURL = strPtr("og_image_url")
	in.Pinned = boolPtr("pinned")
	in.ShowTOC = boolPtr("show_toc")
	if v, ok := raw["tags"]; ok {
		if arr, ok := v.([]any); ok {
			tags := make([]string, 0, len(arr))
			for _, x := range arr {
				if s, ok := x.(string); ok {
					tags = append(tags, s)
				}
			}
			in.Tags = &tags
		}
	}
	if in.ID <= 0 {
		return UpdateInput{}, ErrInvalidInput
	}
	return in, nil
}
