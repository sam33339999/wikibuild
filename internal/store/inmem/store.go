package inmem

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
)

type Store struct {
	mu     sync.RWMutex
	nextID int64
	byID   map[int64]model.Article
	bySlug map[string]int64

	nextUserID  int64
	usersByID   map[int64]model.User
	usersByName map[string]int64

	settings map[string]string

	nextRedirectID int64
	redirects      map[string]model.Redirect // from_path → redirect
}

func New() *Store {
	return &Store{
		byID:        make(map[int64]model.Article),
		bySlug:      make(map[string]int64),
		usersByID:   make(map[int64]model.User),
		usersByName: make(map[string]int64),
		redirects:   make(map[string]model.Redirect),
	}
}

func (s *Store) CreateArticle(ctx context.Context, a model.Article) (model.Article, error) {
	if a.Slug == "" {
		return model.Article{}, store.ErrEmptySlug
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.bySlug[a.Slug]; exists {
		return model.Article{}, store.ErrDuplicateSlug
	}
	s.nextID++
	a.ID = s.nextID
	s.byID[a.ID] = a
	s.bySlug[a.Slug] = a.ID
	return a, nil
}

func (s *Store) GetArticle(ctx context.Context, id int64) (model.Article, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.byID[id]
	if !ok {
		return model.Article{}, store.ErrNotFound
	}
	return a, nil
}

func (s *Store) GetArticleBySlug(ctx context.Context, slug string) (model.Article, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.bySlug[slug]
	if !ok {
		return model.Article{}, store.ErrNotFound
	}
	return s.byID[id], nil
}

func (s *Store) GetArticleByPreviewToken(ctx context.Context, token string) (model.Article, error) {
	if token == "" {
		return model.Article{}, store.ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.byID {
		if a.PreviewToken == token {
			return a, nil
		}
	}
	return model.Article{}, store.ErrNotFound
}

func (s *Store) ListDueScheduled(ctx context.Context, now time.Time) ([]model.Article, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Article
	for _, a := range s.byID {
		if a.Status != model.StatusDraft || a.PublishAt == nil {
			continue
		}
		if !a.PublishAt.After(now) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PublishAt.Before(*out[j].PublishAt)
	})
	return out, nil
}

func (s *Store) CreateRedirect(ctx context.Context, r model.Redirect) (model.Redirect, error) {
	if r.FromPath == "" || r.ToPath == "" {
		return model.Redirect{}, store.ErrEmptyPath
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.redirects[r.FromPath]; ok {
		existing.ToPath = r.ToPath
		s.redirects[r.FromPath] = existing
		return existing, nil
	}
	s.nextRedirectID++
	r.ID = s.nextRedirectID
	s.redirects[r.FromPath] = r
	return r, nil
}

func (s *Store) GetRedirect(ctx context.Context, fromPath string) (model.Redirect, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.redirects[fromPath]
	if !ok {
		return model.Redirect{}, store.ErrNotFound
	}
	return r, nil
}

func (s *Store) ListRedirects(ctx context.Context) ([]model.Redirect, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Redirect, 0, len(s.redirects))
	for _, r := range s.redirects {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (s *Store) DeleteRedirect(ctx context.Context, fromPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.redirects[fromPath]; !ok {
		return store.ErrNotFound
	}
	delete(s.redirects, fromPath)
	return nil
}

func (s *Store) UpdateArticle(ctx context.Context, a model.Article) (model.Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[a.ID]; !ok {
		return model.Article{}, store.ErrNotFound
	}
	if owner, ok := s.bySlug[a.Slug]; ok && owner != a.ID {
		return model.Article{}, store.ErrDuplicateSlug
	}
	if old := s.byID[a.ID]; old.Slug != a.Slug {
		delete(s.bySlug, old.Slug)
	}
	s.byID[a.ID] = a
	s.bySlug[a.Slug] = a.ID
	return a, nil
}

func (s *Store) DeleteArticle(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.byID[id]
	if !ok {
		return store.ErrNotFound
	}
	delete(s.byID, id)
	delete(s.bySlug, a.Slug)
	return nil
}

func (s *Store) ListArticles(ctx context.Context, q store.ListQuery) ([]model.Article, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]model.Article, 0)
	for _, a := range s.byID {
		if q.Status != "" && a.Status != q.Status {
			continue
		}
		if q.Visibility != "" && a.Visibility != q.Visibility {
			continue
		}
		if q.Tag != "" && !containsTag(a.Tags, q.Tag) {
			continue
		}
		if q.Search != "" && !matchesSearch(a, q.Search) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned // pinned articles first
		}
		return out[i].ID > out[j].ID
	})

	total := len(out)
	if q.Offset > 0 {
		if q.Offset >= len(out) {
			return nil, total, nil
		}
		out = out[q.Offset:]
	}
	if q.Limit > 0 && q.Limit < len(out) {
		out = out[:q.Limit]
	}
	return out, total, nil
}

func containsTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// matchesSearch reports whether the article's title or body contains the
// search term (case-insensitive), mirroring the pg layer's ILIKE.
func matchesSearch(a model.Article, term string) bool {
	needle := strings.ToLower(term)
	return strings.Contains(strings.ToLower(a.Title), needle) ||
		strings.Contains(strings.ToLower(a.Body), needle)
}

func (s *Store) CreateUser(ctx context.Context, u model.User) (model.User, error) {
	if u.Username == "" {
		return model.User{}, store.ErrEmptyUsername
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.usersByName[u.Username]; exists {
		return model.User{}, store.ErrDuplicateUsername
	}
	s.nextUserID++
	u.ID = s.nextUserID
	s.usersByID[u.ID] = u
	s.usersByName[u.Username] = u.ID
	return u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usersByName[username]
	if !ok {
		return model.User{}, store.ErrNotFound
	}
	return s.usersByID[id], nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings[key], nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings == nil {
		s.settings = make(map[string]string)
	}
	s.settings[key] = value
	return nil
}

// ListTags aggregates distinct tags across all articles, sorted by name.
func (s *Store) ListTags(ctx context.Context) ([]store.TagCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int)
	for _, a := range s.byID {
		for _, t := range a.Tags {
			if t != "" {
				counts[t]++
			}
		}
	}
	out := make([]store.TagCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, store.TagCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RenameTag renames from→to on every article that carries from. If an
// article already has to, from is removed without introducing a duplicate.
// from == to is a no-op returning 0.
func (s *Store) RenameTag(ctx context.Context, from, to string) (int, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return 0, store.ErrEmptyTag
	}
	if from == to {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, a := range s.byID {
		newTags, changed := rewriteTags(a.Tags, from, to)
		if !changed {
			continue
		}
		a.Tags = newTags
		s.byID[id] = a
		n++
	}
	return n, nil
}

// rewriteTags replaces from with to and drops duplicates. Returns the new
// slice and whether anything changed.
func rewriteTags(tags []string, from, to string) ([]string, bool) {
	if !containsTag(tags, from) {
		return tags, false
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		if t == from {
			t = to
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out, true
}
