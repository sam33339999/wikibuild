package store

import (
	"context"
	"errors"

	"github.com/sam33339999/wikibuild/internal/model"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrDuplicateSlug     = errors.New("duplicate slug")
	ErrDuplicateUsername = errors.New("duplicate username")
	ErrEmptySlug         = errors.New("slug is empty")
	ErrEmptyUsername     = errors.New("username is empty")
	ErrEmptyTag          = errors.New("tag name is empty")
)

// TagCount is one distinct tag and how many articles carry it.
type TagCount struct {
	Name  string
	Count int
}

type Repository interface {
	// Articles
	CreateArticle(ctx context.Context, a model.Article) (model.Article, error)
	GetArticle(ctx context.Context, id int64) (model.Article, error)
	GetArticleBySlug(ctx context.Context, slug string) (model.Article, error)
	UpdateArticle(ctx context.Context, a model.Article) (model.Article, error)
	DeleteArticle(ctx context.Context, id int64) error
	ListArticles(ctx context.Context, q ListQuery) (items []model.Article, total int, err error)

	// Tags (aggregate over article.tags; no separate tags table).
	// ListTags returns distinct tags sorted by name, each with an article count.
	// RenameTag renames from→to on every article that has from (merge-safe:
	// if an article already has to, from is dropped without duplicating).
	// Returns the number of articles updated. Empty names yield ErrEmptyTag.
	ListTags(ctx context.Context) ([]TagCount, error)
	RenameTag(ctx context.Context, from, to string) (int, error)

	// Users
	CreateUser(ctx context.Context, u model.User) (model.User, error)
	GetUserByUsername(ctx context.Context, username string) (model.User, error)

	// Settings (key-value, e.g. default protected password). GetSetting
	// returns "" with a nil error when the key is unset.
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

type ListQuery struct {
	Status     model.Status
	Visibility model.Visibility
	Tag        string
	Search     string
	Limit      int
	Offset     int
}
