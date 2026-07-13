// Package server assembles the Fiber application: middleware (CSRF, recovery)
// and route registration. It wires concrete handlers to the injected store,
// auth primitives and clock, so the same assembly runs in production (pg
// store) and in tests (inmem store).
package server

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/store"
)

// Deps holds the collaborators the server needs to run. Every field is an
// abstraction, so tests build the app with an inmem store and fakes.
type Deps struct {
	Store           store.Repository
	Hasher          auth.PasswordHasher
	Signer          *auth.Signer
	Limiter         *auth.LoginLimiter
	Clock           clock.Clock
	SiteDefaultPass string // fallback password for protected articles without their own
}

// New builds the configured Fiber app. The caller starts it with app.Listen().
func New(d Deps) *fiber.App {
	app := fiber.New()

	app.Use(recover.New())
	// CSRF: double-submit token readable from header or the _csrf form field,
	// so the plain-HTML login form works without JS.
	app.Use(csrf.New(csrf.Config{
		Extractor: extractors.Chain(
			extractors.FromHeader(csrf.HeaderName),
			extractors.FromForm("_csrf"),
		),
	}))

	adminAuth := handler.NewAdminAuth(handler.AdminAuthDeps{
		Store:   d.Store,
		Hasher:  d.Hasher,
		Signer:  d.Signer,
		Limiter: d.Limiter,
		Clock:   d.Clock,
	})
	articleAdmin := handler.NewArticleAdmin(d.Store, d.Hasher)

	// Public auth routes (no auth required).
	app.Get("/admin/login", adminAuth.LoginPage)
	app.Post("/admin/login", adminAuth.LoginSubmit)
	app.Post("/admin/logout", adminAuth.Logout)

	// Authenticated admin article CRUD. The index is registered at the exact
	// static "/admin" path (not the group's trailing-slash "/") so it wins
	// over the public "/:slug" parameter route; sub-routes use the group.
	app.Get("/admin", adminAuth.RequireAuth, articleAdmin.List)
	admin := app.Group("/admin", adminAuth.RequireAuth)
	admin.Get("/new", articleAdmin.NewForm)
	admin.Post("/new", articleAdmin.Create)
	admin.Get("/:id/edit", articleAdmin.EditForm)
	admin.Post("/:id", articleAdmin.Update)
	admin.Post("/:id/delete", articleAdmin.Delete)

	// Public reader-facing pages (registered last; static /admin routes above
	// take priority over the /:slug parameter route).
	pub := handler.NewPublic(d.Store, d.Signer, d.Hasher, d.SiteDefaultPass)
	app.Get("/", pub.Index)
	app.Get("/:slug", pub.Article)
	app.Get("/:slug/unlock", pub.UnlockForm)
	app.Post("/:slug/unlock", pub.UnlockSubmit)

	return app
}
