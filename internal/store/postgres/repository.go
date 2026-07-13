// Package postgres implements store.Repository against PostgreSQL using the
// sqlc-generated Queries. It owns only the model<->sqlc mapping and the
// translation of Postgres errors into store sentinel errors; all SQL lives in
// db/queries (generated into internal/store/sqlc).
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/sqlc"
)

// Postgres constraint names used to map unique violations to sentinel errors.
const (
	constraintArticleSlug  = "articles_slug_key"
	constraintUserUsername = "users_username_key"
)

// Repository wraps sqlc.Queries and implements store.Repository.
type Repository struct {
	q *sqlc.Queries
}

// New builds a Repository backed by the given DBTX (a *pgxpool.Pool or a Tx).
func New(db sqlc.DBTX) *Repository {
	return &Repository{q: sqlc.New(db)}
}

// ---------------- Articles ----------------

func (r *Repository) CreateArticle(ctx context.Context, a model.Article) (model.Article, error) {
	if a.Slug == "" {
		return model.Article{}, store.ErrEmptySlug
	}
	row, err := r.q.CreateArticle(ctx, sqlc.CreateArticleParams{
		Slug:        a.Slug,
		Title:       a.Title,
		Type:        string(a.Type),
		Status:      string(a.Status),
		Visibility:  string(a.Visibility),
		Password:    a.Password,
		RawMode:     a.RawMode,
		Body:        a.Body,
		Tags:        normalizeTags(a.Tags),
		CreatedAt:   nargTime(a.CreatedAt),
		UpdatedAt:   nargTime(a.UpdatedAt),
		PublishedAt: toTimestamptz(a.PublishedAt),
	})
	if err != nil {
		return model.Article{}, mapArticleErr(err)
	}
	return articleToModel(row), nil
}

func (r *Repository) GetArticle(ctx context.Context, id int64) (model.Article, error) {
	row, err := r.q.GetArticle(ctx, id)
	if err != nil {
		return model.Article{}, mapArticleErr(err)
	}
	return articleToModel(row), nil
}

func (r *Repository) GetArticleBySlug(ctx context.Context, slug string) (model.Article, error) {
	row, err := r.q.GetArticleBySlug(ctx, slug)
	if err != nil {
		return model.Article{}, mapArticleErr(err)
	}
	return articleToModel(row), nil
}

func (r *Repository) UpdateArticle(ctx context.Context, a model.Article) (model.Article, error) {
	row, err := r.q.UpdateArticle(ctx, sqlc.UpdateArticleParams{
		ID:          a.ID,
		Slug:        a.Slug,
		Title:       a.Title,
		Type:        string(a.Type),
		Status:      string(a.Status),
		Visibility:  string(a.Visibility),
		Password:    a.Password,
		RawMode:     a.RawMode,
		Body:        a.Body,
		Tags:        normalizeTags(a.Tags),
		UpdatedAt:   toTimestamptz(nonZeroPtr(a.UpdatedAt)),
		PublishedAt: toTimestamptz(a.PublishedAt),
	})
	if err != nil {
		return model.Article{}, mapArticleErr(err)
	}
	return articleToModel(row), nil
}

func (r *Repository) DeleteArticle(ctx context.Context, id int64) error {
	if err := r.q.DeleteArticle(ctx, id); err != nil {
		return mapArticleErr(err)
	}
	return nil
}

func (r *Repository) ListArticles(ctx context.Context, q store.ListQuery) ([]model.Article, int, error) {
	params := sqlc.ListArticlesParams{
		Status:     toText(string(q.Status)),
		Visibility: toText(string(q.Visibility)),
		Tag:        toText(q.Tag),
		Search:     q.Search,
		Skip:       int32(q.Offset),
	}
	if q.Limit > 0 {
		params.MaxRows = int32(q.Limit)
	}
	rows, err := r.q.ListArticles(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	items := make([]model.Article, 0, len(rows))
	for _, row := range rows {
		items = append(items, articleToModel(row))
	}

	total, err := r.q.CountArticles(ctx, sqlc.CountArticlesParams{
		Status:     params.Status,
		Visibility: params.Visibility,
		Tag:        params.Tag,
		Search:     params.Search,
	})
	if err != nil {
		return nil, 0, err
	}
	return items, int(total), nil
}

// ---------------- Users ----------------

func (r *Repository) CreateUser(ctx context.Context, u model.User) (model.User, error) {
	if u.Username == "" {
		return model.User{}, store.ErrEmptyUsername
	}
	row, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    nargTime(u.CreatedAt),
	})
	if err != nil {
		return model.User{}, mapUserErr(err)
	}
	return userToModel(row), nil
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (model.User, error) {
	row, err := r.q.GetUserByUsername(ctx, username)
	if err != nil {
		return model.User{}, mapUserErr(err)
	}
	return userToModel(row), nil
}

// ---------------- Settings ----------------

// GetSetting returns the value for key, or "" with a nil error when unset.
func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	val, err := r.q.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// SetSetting upserts a setting value.
func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.q.SetSetting(ctx, sqlc.SetSettingParams{Key: key, Value: value})
	return err
}

// ---------------- error mapping ----------------

func mapArticleErr(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return store.ErrNotFound
	default:
		return mapUnique(err, constraintArticleSlug, store.ErrDuplicateSlug)
	}
}

func mapUserErr(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return store.ErrNotFound
	default:
		return mapUnique(err, constraintUserUsername, store.ErrDuplicateUsername)
	}
}

// mapUnique converts a Postgres unique-violation (SQLSTATE 23505) on the given
// constraint into the supplied sentinel error; other errors pass through.
func mapUnique(err error, constraint string, sentinel error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint {
		return sentinel
	}
	return err
}

// ---------------- conversions ----------------

func articleToModel(a sqlc.Article) model.Article {
	return model.Article{
		ID:          a.ID,
		Slug:        a.Slug,
		Title:       a.Title,
		Type:        model.ArticleType(a.Type),
		Status:      model.Status(a.Status),
		Visibility:  model.Visibility(a.Visibility),
		Password:    a.Password,
		RawMode:     a.RawMode,
		Body:        a.Body,
		Tags:        a.Tags,
		CreatedAt:   fromTimestamptz(a.CreatedAt),
		UpdatedAt:   fromTimestamptz(a.UpdatedAt),
		PublishedAt: fromTimestamptzPtr(a.PublishedAt),
	}
}

func userToModel(u sqlc.User) model.User {
	return model.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    fromTimestamptz(u.CreatedAt),
	}
}

// nargTime returns a value suitable for COALESCE(narg, now()): a zero time
// becomes nil (use the DB default), otherwise a Timestamptz is sent.
func nargTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// nonZeroPtr returns a *time.Time only for a non-zero time, so optional
// UpdatedAt can fall back to now() via COALESCE.
func nonZeroPtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func toTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func toText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// normalizeTags returns a non-nil slice so pgx encodes '{}' (not NULL) for the
// NOT NULL tags column. The model layer may carry nil for "no tags".
func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

func fromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func fromTimestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tt := t.Time
	return &tt
}
