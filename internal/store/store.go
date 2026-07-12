package store

import (
	"context"
	"errors"

	"github.com/sam33339999/wikibuild/internal/model"
)

var (
	ErrNotFound      = errors.New("article not found")
	ErrDuplicateSlug = errors.New("duplicate slug")
	ErrEmptySlug     = errors.New("slug is empty")
)

type Repository interface {
	CreateArticle(ctx context.Context, a model.Article) (model.Article, error)
	GetArticle(ctx context.Context, id int64) (model.Article, error)
	GetArticleBySlug(ctx context.Context, slug string) (model.Article, error)
	UpdateArticle(ctx context.Context, a model.Article) (model.Article, error)
	DeleteArticle(ctx context.Context, id int64) error
	ListArticles(ctx context.Context, q ListQuery) (items []model.Article, total int, err error)
}

type ListQuery struct {
	Status     model.Status
	Visibility model.Visibility
	Tag        string
	Search     string
	Limit      int
	Offset     int
}
