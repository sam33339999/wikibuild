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
	ID          int64
	Slug        string
	Title       string
	Type        ArticleType
	Status      Status
	Visibility  Visibility
	Password    string
	RawMode     bool
	Pinned      bool
	Body        string
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt *time.Time
}
