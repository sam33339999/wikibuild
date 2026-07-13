package inmem

import (
	"context"
	"sort"
	"sync"

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
}

func New() *Store {
	return &Store{
		byID:        make(map[int64]model.Article),
		bySlug:      make(map[string]int64),
		usersByID:   make(map[int64]model.User),
		usersByName: make(map[string]int64),
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
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })

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
