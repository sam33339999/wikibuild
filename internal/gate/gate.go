// Package gate decides whether a reader may view an article based on its
// visibility, the reader's admin status, and a per-article unlock cookie.
//
// The package is pure (no I/O): it turns request context into a Decision,
// which the HTTP layer translates into a response. This keeps the visibility
// rules trivially unit-testable and consistent across routes.
package gate

import (
	"github.com/sam33339999/wikibuild/internal/model"
)

// Decision is what the HTTP layer should do for a given article request.
type Decision int

const (
	// Allow renders the article.
	Allow Decision = iota
	// NotFound returns 404 — used for private articles seen by non-admins
	// (and drafts), so existence is never leaked.
	NotFound
	// Password renders the protected-article unlock form.
	Password
)

// AccessInput summarises what the handler knows about the request: the
// article's status/visibility, whether the reader is an authenticated admin,
// and whether a valid per-article unlock cookie was presented.
type AccessInput struct {
	Status     model.Status
	Visibility model.Visibility
	IsAdmin    bool
	Unlocked   bool
}

// Decide applies the visibility rules from the README:
//
//   - public  → anyone
//   - private → admin only; non-admins get 404 (not 403)
//   - protected → admin, or a valid unlock cookie; otherwise the password page
//
// Drafts are never shown to non-admins (404), regardless of visibility, so
// unpublished work stays hidden.
func Decide(in AccessInput) Decision {
	// Drafts are invisible to non-admins.
	if in.Status != model.StatusPublished && !in.IsAdmin {
		return NotFound
	}

	switch in.Visibility {
	case model.VisibilityPublic:
		return Allow
	case model.VisibilityPrivate:
		if in.IsAdmin {
			return Allow
		}
		return NotFound
	case model.VisibilityProtected:
		if in.IsAdmin || in.Unlocked {
			return Allow
		}
		return Password
	default:
		return NotFound
	}
}
