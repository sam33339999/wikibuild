package model

import "time"

type ArticleType string

const (
	ArticleTypeMarkdown   ArticleType = "markdown"
	ArticleTypeHTMLUpload ArticleType = "html_upload"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
)

type Visibility string

const (
	VisibilityPublic    Visibility = "public"
	VisibilityProtected Visibility = "protected"
	VisibilityPrivate   Visibility = "private"
)

type Article struct {
	ID           int64
	Slug         string
	Title        string
	Type         ArticleType
	Status       Status
	Visibility   Visibility
	Password     string
	RawMode      bool
	Pinned       bool
	ShowTOC      bool // markdown: render collapsible TOC sidebar (default true)
	Body         string
	Tags         []string
	// SEO / social (empty = automatic fallbacks on public render).
	SEOTitle        string // <title> / og:title if set; else Title
	Summary         string // human/AI blurb; feeds prefer this
	MetaDescription string // meta description / og:description if set
	CoverImageURL   string // optional on-page hero / list card
	OGImageURL      string // OG/Twitter image; falls back to CoverImageURL
	CreatedAt    time.Time
	UpdatedAt    time.Time
	PublishedAt  *time.Time
	PublishAt    *time.Time // scheduled publish; draft until due
	PreviewToken string     // unlisted draft share link token
}

// Redirect is a permanent (301) path mapping, typically created when an
// article slug changes so old URLs keep working.
type Redirect struct {
	ID        int64
	FromPath  string
	ToPath    string
	CreatedAt time.Time
}
