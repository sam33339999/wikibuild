// Package scheduler publishes drafts whose PublishAt has arrived.
// Pure logic over store.Repository + clock; the process loop lives in main.
package scheduler

import (
	"context"
	"time"

	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
)

// Publisher flips due scheduled drafts to published.
type Publisher struct {
	Repo  store.Repository
	Clock clock.Clock
}

// Tick publishes every draft with PublishAt <= now. Returns how many articles
// were published. Each update sets Status=published, PublishedAt=now (or the
// scheduled time if preferred — we use now for "actually went live"), and
// clears PublishAt so it does not re-fire.
func (p *Publisher) Tick(ctx context.Context) (int, error) {
	now := p.Clock.Now().UTC()
	due, err := p.Repo.ListDueScheduled(ctx, now)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range due {
		a.Status = model.StatusPublished
		pub := now
		if a.PublishAt != nil {
			// Use the scheduled time as PublishedAt so archives reflect intent.
			pub = a.PublishAt.UTC()
		}
		a.PublishedAt = &pub
		a.PublishAt = nil
		if _, err := p.Repo.UpdateArticle(ctx, a); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// DefaultInterval is how often main should call Tick.
const DefaultInterval = time.Minute
