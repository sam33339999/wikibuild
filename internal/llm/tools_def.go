package llm

// ToolDef is an OpenAI-compatible tools[] entry (type=function).
type ToolDef struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

// ToolFunctionSchema describes one callable function.
type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall is one model-requested function invocation.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON object as string
}

// ChatResult is a non-stream chat/completions outcome.
type ChatResult struct {
	Content   string
	ToolCalls []ToolCall
	// FinishReason e.g. stop | tool_calls
	FinishReason string
}

// ArticleToolDefs returns OpenAI tool schemas matching WikiBuild MCP article tools.
func ArticleToolDefs() []ToolDef {
	return []ToolDef{
		fn("list_articles", "List articles with optional filters (status, visibility, search q).", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":     map[string]any{"type": "string", "description": "draft or published"},
				"visibility": map[string]any{"type": "string", "description": "public, protected, or private"},
				"q":          map[string]any{"type": "string", "description": "search title/body"},
				"limit":      map[string]any{"type": "integer"},
				"offset":     map[string]any{"type": "integer"},
			},
		}),
		fn("get_article", "Get one article by id or slug (includes body).", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "integer"},
				"slug": map[string]any{"type": "string"},
			},
		}),
		fn("create_article", "Create a markdown article. Defaults: draft + private.", map[string]any{
			"type":                 "object",
			"required":             []string{"slug", "title"},
			"properties": map[string]any{
				"slug":             map[string]any{"type": "string"},
				"title":            map[string]any{"type": "string"},
				"body":             map[string]any{"type": "string"},
				"status":           map[string]any{"type": "string"},
				"visibility":       map[string]any{"type": "string"},
				"seo_title":        map[string]any{"type": "string"},
				"summary":          map[string]any{"type": "string"},
				"meta_description": map[string]any{"type": "string"},
				"cover_image_url":  map[string]any{"type": "string"},
				"og_image_url":     map[string]any{"type": "string"},
				"pinned":           map[string]any{"type": "boolean"},
			},
		}),
		fn("update_article", "Patch article fields by id.", map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":               map[string]any{"type": "integer"},
				"slug":             map[string]any{"type": "string"},
				"title":            map[string]any{"type": "string"},
				"body":             map[string]any{"type": "string"},
				"seo_title":        map[string]any{"type": "string"},
				"summary":          map[string]any{"type": "string"},
				"meta_description": map[string]any{"type": "string"},
				"cover_image_url":  map[string]any{"type": "string"},
				"og_image_url":     map[string]any{"type": "string"},
				"pinned":           map[string]any{"type": "boolean"},
				"show_toc":         map[string]any{"type": "boolean"},
			},
		}),
		fn("set_article_status", "Set status to draft or published.", map[string]any{
			"type":     "object",
			"required": []string{"id", "status"},
			"properties": map[string]any{
				"id":         map[string]any{"type": "integer"},
				"status":     map[string]any{"type": "string"},
				"publish_at": map[string]any{"type": "string", "description": "RFC3339 for scheduled draft"},
			},
		}),
		fn("set_article_visibility", "Set visibility public/protected/private.", map[string]any{
			"type":     "object",
			"required": []string{"id", "visibility"},
			"properties": map[string]any{
				"id":         map[string]any{"type": "integer"},
				"visibility": map[string]any{"type": "string"},
			},
		}),
	}
}

func fn(name, desc string, params map[string]any) ToolDef {
	return ToolDef{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        name,
			Description: desc,
			Parameters:  params,
		},
	}
}
