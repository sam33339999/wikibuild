package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sam33339999/wikibuild/internal/store"
)

// NewServer builds an MCP server with article tools bound to tools.
func NewServer(tools *Tools) *server.MCPServer {
	s := server.NewMCPServer(
		"wikibuild",
		"1.1.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(mcplib.NewTool("list_articles",
		mcplib.WithDescription("List WikiBuild articles with optional status/visibility/q filters and pagination."),
		mcplib.WithString("status", mcplib.Description("draft or published")),
		mcplib.WithString("visibility", mcplib.Description("public, protected, or private")),
		mcplib.WithString("q", mcplib.Description("search title/body")),
		mcplib.WithNumber("limit", mcplib.Description("max rows (default 50)")),
		mcplib.WithNumber("offset", mcplib.Description("skip rows")),
	), func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		var in ListInput
		if err := req.BindArguments(&in); err != nil {
			// fallback: map from Get* helpers
			in = ListInput{
				Status:     req.GetString("status", ""),
				Visibility: req.GetString("visibility", ""),
				Q:          req.GetString("q", ""),
				Limit:      req.GetInt("limit", 0),
				Offset:     req.GetInt("offset", 0),
			}
		}
		items, err := tools.ListArticles(ctx, in)
		return jsonOrErr(items, err)
	})

	s.AddTool(mcplib.NewTool("get_article",
		mcplib.WithDescription("Get one article by id or slug (full body)."),
		mcplib.WithNumber("id", mcplib.Description("article id")),
		mcplib.WithString("slug", mcplib.Description("article slug")),
	), func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		in := GetInput{
			ID:   int64(req.GetInt("id", 0)),
			Slug: req.GetString("slug", ""),
		}
		item, err := tools.GetArticle(ctx, in)
		return jsonOrErr(item, err)
	})

	s.AddTool(mcplib.NewTool("create_article",
		mcplib.WithDescription("Create a markdown article. Defaults: status=draft, visibility=private."),
		mcplib.WithString("slug", mcplib.Required(), mcplib.Description("URL slug")),
		mcplib.WithString("title", mcplib.Required(), mcplib.Description("display title")),
		mcplib.WithString("body", mcplib.Description("markdown body")),
		mcplib.WithString("status", mcplib.Description("draft or published (default draft)")),
		mcplib.WithString("visibility", mcplib.Description("public/protected/private (default private)")),
		mcplib.WithString("seo_title", mcplib.Description("optional SEO title")),
		mcplib.WithString("summary", mcplib.Description("optional summary")),
		mcplib.WithString("meta_description", mcplib.Description("optional meta description")),
		mcplib.WithString("cover_image_url", mcplib.Description("optional cover URL")),
		mcplib.WithString("og_image_url", mcplib.Description("optional OG image URL")),
		mcplib.WithBoolean("pinned", mcplib.Description("pin on home")),
		mcplib.WithBoolean("show_toc", mcplib.Description("show TOC (default true)")),
	), func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		in := CreateInput{
			Slug:            req.GetString("slug", ""),
			Title:           req.GetString("title", ""),
			Body:            req.GetString("body", ""),
			Tags:            req.GetStringSlice("tags", nil),
			Status:          req.GetString("status", ""),
			Visibility:      req.GetString("visibility", ""),
			SEOTitle:        req.GetString("seo_title", ""),
			Summary:         req.GetString("summary", ""),
			MetaDescription: req.GetString("meta_description", ""),
			CoverImageURL:   req.GetString("cover_image_url", ""),
			OGImageURL:      req.GetString("og_image_url", ""),
			Pinned:          req.GetBool("pinned", false),
		}
		if args := req.GetArguments(); args != nil {
			if _, ok := args["show_toc"]; ok {
				v := req.GetBool("show_toc", true)
				in.ShowTOC = &v
			}
		}
		item, err := tools.CreateArticle(ctx, in)
		return jsonOrErr(item, err)
	})

	s.AddTool(mcplib.NewTool("update_article",
		mcplib.WithDescription("Patch article fields by id. Omitted fields stay unchanged."),
		mcplib.WithNumber("id", mcplib.Required(), mcplib.Description("article id")),
		mcplib.WithString("slug", mcplib.Description("new slug")),
		mcplib.WithString("title", mcplib.Description("new title")),
		mcplib.WithString("body", mcplib.Description("new markdown body")),
		mcplib.WithString("seo_title", mcplib.Description("SEO title")),
		mcplib.WithString("summary", mcplib.Description("summary")),
		mcplib.WithString("meta_description", mcplib.Description("meta description")),
		mcplib.WithString("cover_image_url", mcplib.Description("cover URL")),
		mcplib.WithString("og_image_url", mcplib.Description("OG image URL")),
		mcplib.WithBoolean("pinned", mcplib.Description("pinned")),
		mcplib.WithBoolean("show_toc", mcplib.Description("show TOC")),
	), func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		in := UpdateInput{ID: int64(req.GetInt("id", 0))}
		args := req.GetArguments()
		if args == nil {
			args = map[string]any{}
		}
		setStr := func(key string, dst **string) {
			if _, ok := args[key]; ok {
				v := req.GetString(key, "")
				*dst = &v
			}
		}
		setStr("slug", &in.Slug)
		setStr("title", &in.Title)
		setStr("body", &in.Body)
		setStr("seo_title", &in.SEOTitle)
		setStr("summary", &in.Summary)
		setStr("meta_description", &in.MetaDescription)
		setStr("cover_image_url", &in.CoverImageURL)
		setStr("og_image_url", &in.OGImageURL)
		if _, ok := args["pinned"]; ok {
			v := req.GetBool("pinned", false)
			in.Pinned = &v
		}
		if _, ok := args["show_toc"]; ok {
			v := req.GetBool("show_toc", true)
			in.ShowTOC = &v
		}
		if tags := req.GetStringSlice("tags", nil); tags != nil {
			if _, ok := args["tags"]; ok {
				in.Tags = &tags
			}
		}
		item, err := tools.UpdateArticle(ctx, in)
		return jsonOrErr(item, err)
	})

	s.AddTool(mcplib.NewTool("set_article_status",
		mcplib.WithDescription("Set article status to draft or published. Optional publish_at (RFC3339) for scheduled drafts."),
		mcplib.WithNumber("id", mcplib.Required(), mcplib.Description("article id")),
		mcplib.WithString("status", mcplib.Required(), mcplib.Description("draft or published")),
		mcplib.WithString("publish_at", mcplib.Description("RFC3339 schedule time for draft")),
	), func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		in := SetStatusInput{
			ID:        int64(req.GetInt("id", 0)),
			Status:    req.GetString("status", ""),
			PublishAt: req.GetString("publish_at", ""),
		}
		item, err := tools.SetArticleStatus(ctx, in)
		return jsonOrErr(item, err)
	})

	s.AddTool(mcplib.NewTool("set_article_visibility",
		mcplib.WithDescription("Set article visibility: public, protected, or private."),
		mcplib.WithNumber("id", mcplib.Required(), mcplib.Description("article id")),
		mcplib.WithString("visibility", mcplib.Required(), mcplib.Description("public, protected, or private")),
	), func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		in := SetVisibilityInput{
			ID:         int64(req.GetInt("id", 0)),
			Visibility: req.GetString("visibility", ""),
		}
		item, err := tools.SetArticleVisibility(ctx, in)
		return jsonOrErr(item, err)
	})

	return s
}

// ServeStdio runs the MCP server on stdin/stdout until the client disconnects.
func ServeStdio(tools *Tools) error {
	return server.ServeStdio(NewServer(tools))
}

func jsonOrErr(v any, err error) (*mcplib.CallToolResult, error) {
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcplib.NewToolResultError("not found"), nil
		}
		if errors.Is(err, ErrInvalidInput) {
			return mcplib.NewToolResultError("invalid input"), nil
		}
		if errors.Is(err, store.ErrDuplicateSlug) {
			return mcplib.NewToolResultError("duplicate slug"), nil
		}
		if errors.Is(err, store.ErrEmptySlug) {
			return mcplib.NewToolResultError("slug is required"), nil
		}
		return mcplib.NewToolResultError(err.Error()), nil
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcplib.NewToolResultText(string(raw)), nil
}
