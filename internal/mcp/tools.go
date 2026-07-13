// Package mcp implements article tools for the WikiBuild MCP server.
// Business logic is unit-tested against store.Repository (inmem); the stdio
// transport is a thin adapter over these tools.
package mcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
)

// Sentinel errors for tool input / auth.
var (
	ErrInvalidInput = errors.New("mcp: invalid input")
	ErrUnauthorized = errors.New("mcp: unauthorized")
)

// ValidateToken rejects empty MCP auth tokens (fail closed).
func ValidateToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrUnauthorized
	}
	return nil
}

// Tools exposes article CRUD for MCP agents. No password hashes in views.
type Tools struct {
	repo  store.Repository
	clock clock.Clock
}

// NewTools builds a Tools service. clk may be nil (uses clock.Real).
func NewTools(repo store.Repository, clk clock.Clock) *Tools {
	if clk == nil {
		clk = clock.Real{}
	}
	return &Tools{repo: repo, clock: clk}
}

// ArticleView is a safe projection for MCP responses (no password hash).
type ArticleView struct {
	ID              int64    `json:"id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Visibility      string   `json:"visibility"`
	Body            string   `json:"body,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	SEOTitle        string   `json:"seo_title,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	MetaDescription string   `json:"meta_description,omitempty"`
	CoverImageURL   string   `json:"cover_image_url,omitempty"`
	OGImageURL      string   `json:"og_image_url,omitempty"`
	Pinned          bool     `json:"pinned,omitempty"`
	ShowTOC         bool     `json:"show_toc,omitempty"`
	// Password intentionally omitted.
	PublishedAt *time.Time `json:"published_at,omitempty"`
	PublishAt   *time.Time `json:"publish_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at,omitempty"`
}

// ToView maps a model.Article to a password-free view.
func (t *Tools) ToView(a model.Article) ArticleView {
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	return ArticleView{
		ID:              a.ID,
		Slug:            a.Slug,
		Title:           a.Title,
		Type:            string(a.Type),
		Status:          string(a.Status),
		Visibility:      string(a.Visibility),
		Body:            a.Body,
		Tags:            tags,
		SEOTitle:        a.SEOTitle,
		Summary:         a.Summary,
		MetaDescription: a.MetaDescription,
		CoverImageURL:   a.CoverImageURL,
		OGImageURL:      a.OGImageURL,
		Pinned:          a.Pinned,
		ShowTOC:         a.ShowTOC,
		PublishedAt:     a.PublishedAt,
		PublishAt:       a.PublishAt,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}

// ListInput filters for list_articles.
type ListInput struct {
	Status     string `json:"status"`
	Visibility string `json:"visibility"`
	Q          string `json:"q"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

// ListArticles returns article views (newest first via store).
func (t *Tools) ListArticles(ctx context.Context, in ListInput) ([]ArticleView, error) {
	q := store.ListQuery{
		Search: strings.TrimSpace(in.Q),
		Limit:  in.Limit,
		Offset: in.Offset,
	}
	if in.Limit <= 0 {
		q.Limit = 50
	}
	if s := strings.TrimSpace(in.Status); s != "" {
		if !validStatus(s) {
			return nil, ErrInvalidInput
		}
		q.Status = model.Status(s)
	}
	if v := strings.TrimSpace(in.Visibility); v != "" {
		if !validVisibility(v) {
			return nil, ErrInvalidInput
		}
		q.Visibility = model.Visibility(v)
	}
	items, _, err := t.repo.ListArticles(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]ArticleView, 0, len(items))
	for _, a := range items {
		// List omits body for size; agents use get_article for full content.
		v := t.ToView(a)
		v.Body = ""
		out = append(out, v)
	}
	return out, nil
}

// GetInput identifies an article by id or slug.
type GetInput struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
}

// GetArticle loads one article by id or slug.
func (t *Tools) GetArticle(ctx context.Context, in GetInput) (ArticleView, error) {
	if in.ID > 0 {
		a, err := t.repo.GetArticle(ctx, in.ID)
		if err != nil {
			return ArticleView{}, err
		}
		return t.ToView(a), nil
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		return ArticleView{}, ErrInvalidInput
	}
	a, err := t.repo.GetArticleBySlug(ctx, slug)
	if err != nil {
		return ArticleView{}, err
	}
	return t.ToView(a), nil
}

// CreateInput creates a markdown article. Defaults: draft + private.
type CreateInput struct {
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	Tags            []string `json:"tags"`
	Status          string   `json:"status"`     // optional override
	Visibility      string   `json:"visibility"` // optional override
	SEOTitle        string   `json:"seo_title"`
	Summary         string   `json:"summary"`
	MetaDescription string   `json:"meta_description"`
	CoverImageURL   string   `json:"cover_image_url"`
	OGImageURL      string   `json:"og_image_url"`
	Pinned          bool     `json:"pinned"`
	ShowTOC         *bool    `json:"show_toc"` // nil → true
}

// CreateArticle inserts a new markdown article.
func (t *Tools) CreateArticle(ctx context.Context, in CreateInput) (ArticleView, error) {
	status := model.StatusDraft
	if s := strings.TrimSpace(in.Status); s != "" {
		if !validStatus(s) {
			return ArticleView{}, ErrInvalidInput
		}
		status = model.Status(s)
	}
	vis := model.VisibilityPrivate
	if v := strings.TrimSpace(in.Visibility); v != "" {
		if !validVisibility(v) {
			return ArticleView{}, ErrInvalidInput
		}
		vis = model.Visibility(v)
	}
	showTOC := true
	if in.ShowTOC != nil {
		showTOC = *in.ShowTOC
	}
	a := model.Article{
		Slug:            strings.TrimSpace(in.Slug),
		Title:           strings.TrimSpace(in.Title),
		Body:            in.Body,
		Tags:            in.Tags,
		Type:            model.ArticleTypeMarkdown,
		Status:          status,
		Visibility:      vis,
		SEOTitle:        strings.TrimSpace(in.SEOTitle),
		Summary:         strings.TrimSpace(in.Summary),
		MetaDescription: strings.TrimSpace(in.MetaDescription),
		CoverImageURL:   strings.TrimSpace(in.CoverImageURL),
		OGImageURL:      strings.TrimSpace(in.OGImageURL),
		Pinned:          in.Pinned,
		ShowTOC:         showTOC,
	}
	stampPublishedAt(&a, nil, t.clock.Now())
	created, err := t.repo.CreateArticle(ctx, a)
	if err != nil {
		return ArticleView{}, err
	}
	return t.ToView(created), nil
}

// UpdateInput patches selected fields (nil pointer = leave unchanged).
type UpdateInput struct {
	ID              int64     `json:"id"`
	Slug            *string   `json:"slug"`
	Title           *string   `json:"title"`
	Body            *string   `json:"body"`
	Tags            *[]string `json:"tags"`
	SEOTitle        *string   `json:"seo_title"`
	Summary         *string   `json:"summary"`
	MetaDescription *string   `json:"meta_description"`
	CoverImageURL   *string   `json:"cover_image_url"`
	OGImageURL      *string   `json:"og_image_url"`
	Pinned          *bool     `json:"pinned"`
	ShowTOC         *bool     `json:"show_toc"`
}

// UpdateArticle applies a patch and returns the updated view.
func (t *Tools) UpdateArticle(ctx context.Context, in UpdateInput) (ArticleView, error) {
	if in.ID <= 0 {
		return ArticleView{}, ErrInvalidInput
	}
	a, err := t.repo.GetArticle(ctx, in.ID)
	if err != nil {
		return ArticleView{}, err
	}
	if in.Slug != nil {
		a.Slug = strings.TrimSpace(*in.Slug)
	}
	if in.Title != nil {
		a.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body != nil {
		a.Body = *in.Body
	}
	if in.Tags != nil {
		a.Tags = *in.Tags
	}
	if in.SEOTitle != nil {
		a.SEOTitle = strings.TrimSpace(*in.SEOTitle)
	}
	if in.Summary != nil {
		a.Summary = strings.TrimSpace(*in.Summary)
	}
	if in.MetaDescription != nil {
		a.MetaDescription = strings.TrimSpace(*in.MetaDescription)
	}
	if in.CoverImageURL != nil {
		a.CoverImageURL = strings.TrimSpace(*in.CoverImageURL)
	}
	if in.OGImageURL != nil {
		a.OGImageURL = strings.TrimSpace(*in.OGImageURL)
	}
	if in.Pinned != nil {
		a.Pinned = *in.Pinned
	}
	if in.ShowTOC != nil {
		a.ShowTOC = *in.ShowTOC
	}
	updated, err := t.repo.UpdateArticle(ctx, a)
	if err != nil {
		return ArticleView{}, err
	}
	return t.ToView(updated), nil
}

// SetStatusInput changes draft/published (+ optional schedule).
type SetStatusInput struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	PublishAt string `json:"publish_at"` // optional RFC3339 for draft schedule
}

// SetArticleStatus updates status and PublishedAt / PublishAt.
func (t *Tools) SetArticleStatus(ctx context.Context, in SetStatusInput) (ArticleView, error) {
	if in.ID <= 0 || !validStatus(in.Status) {
		return ArticleView{}, ErrInvalidInput
	}
	a, err := t.repo.GetArticle(ctx, in.ID)
	if err != nil {
		return ArticleView{}, err
	}
	existing := a
	a.Status = model.Status(strings.TrimSpace(in.Status))
	if raw := strings.TrimSpace(in.PublishAt); raw != "" && a.Status == model.StatusDraft {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return ArticleView{}, ErrInvalidInput
		}
		tt := t.UTC()
		a.PublishAt = &tt
	}
	stampPublishedAt(&a, &existing, t.clock.Now())
	updated, err := t.repo.UpdateArticle(ctx, a)
	if err != nil {
		return ArticleView{}, err
	}
	return t.ToView(updated), nil
}

// SetVisibilityInput changes visibility.
type SetVisibilityInput struct {
	ID         int64  `json:"id"`
	Visibility string `json:"visibility"`
}

// SetArticleVisibility updates visibility only.
func (t *Tools) SetArticleVisibility(ctx context.Context, in SetVisibilityInput) (ArticleView, error) {
	if in.ID <= 0 || !validVisibility(in.Visibility) {
		return ArticleView{}, ErrInvalidInput
	}
	a, err := t.repo.GetArticle(ctx, in.ID)
	if err != nil {
		return ArticleView{}, err
	}
	a.Visibility = model.Visibility(strings.TrimSpace(in.Visibility))
	updated, err := t.repo.UpdateArticle(ctx, a)
	if err != nil {
		return ArticleView{}, err
	}
	return t.ToView(updated), nil
}

func validStatus(s string) bool {
	switch model.Status(strings.TrimSpace(s)) {
	case model.StatusDraft, model.StatusPublished:
		return true
	default:
		return false
	}
}

func validVisibility(s string) bool {
	switch model.Visibility(strings.TrimSpace(s)) {
	case model.VisibilityPublic, model.VisibilityProtected, model.VisibilityPrivate:
		return true
	default:
		return false
	}
}

// stampPublishedAt mirrors handler semantics for MCP status changes.
func stampPublishedAt(a *model.Article, existing *model.Article, now time.Time) {
	if a.Status != model.StatusPublished {
		a.PublishedAt = nil
		return
	}
	a.PublishAt = nil
	if existing != nil && existing.PublishedAt != nil {
		a.PublishedAt = existing.PublishedAt
		return
	}
	t := now.UTC()
	a.PublishedAt = &t
}
