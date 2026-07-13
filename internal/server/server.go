// Package server assembles the Fiber application: middleware (CSRF, recovery)
// and route registration. It wires concrete handlers to the injected store,
// auth primitives and clock, so the same assembly runs in production (pg
// store) and in tests (inmem store).
package server

import (
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/media"
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
	ContentDir      string // root for html_upload article files
	MediaDir        string // root for pasted/dragged images; empty → sibling of ContentDir
	StaticDir       string // CSS/JS theme assets; empty → ./static
	BaseURL         string // absolute site origin for feeds/sitemap/SEO (no trailing slash)
	SiteTitle       string // feed channel title
}

// mediaDir resolves the on-disk image directory: explicit MediaDir wins,
// otherwise content/media next to content/uploads.
func mediaDir(d Deps) string {
	if d.MediaDir != "" {
		return d.MediaDir
	}
	if d.ContentDir == "" {
		return "content/media"
	}
	return filepath.Join(filepath.Dir(d.ContentDir), "media")
}

// New builds the configured Fiber app. The caller starts it with app.Listen().
func New(d Deps) *fiber.App {
	// BodyLimit must exceed media.MaxBytes so the image handler can reject
	// oversized uploads with a typed error rather than Fiber's generic 413.
	app := fiber.New(fiber.Config{
		BodyLimit: media.MaxBytes + 512*1024,
	})

	app.Use(recover.New())
	// CSRF: double-submit token readable from header or the _csrf form field,
	// so the plain-HTML login form works without JS.
	app.Use(csrf.New(csrf.Config{
		Extractor: extractors.Chain(
			extractors.FromHeader(csrf.HeaderName),
			extractors.FromForm("_csrf"),
		),
	}))

	// Theme CSS/JS (no build step). Registered early so /static is never
	// captured by /:slug.
	staticDir := d.StaticDir
	if staticDir == "" {
		staticDir = "./static"
	}
	app.Get("/static/*", static.New(staticDir, static.Config{MaxAge: 86400}))

	adminAuth := handler.NewAdminAuth(handler.AdminAuthDeps{
		Store:   d.Store,
		Hasher:  d.Hasher,
		Signer:  d.Signer,
		Limiter: d.Limiter,
		Clock:   d.Clock,
	})
	articleAdmin := handler.NewArticleAdmin(d.Store, d.Hasher, d.Clock, d.ContentDir)
	settings := handler.NewSettings(d.Store)
	uploads := handler.NewUpload(d.Store, d.ContentDir, d.Hasher)
	mediaH := handler.NewMedia(mediaDir(d))
	tags := handler.NewTags(d.Store)
	redirects := handler.NewRedirects(d.Store)
	baseURL := strings.TrimRight(d.BaseURL, "/")
	syn := handler.NewSyndication(d.Store, baseURL, d.SiteTitle)

	// Public media (no auth): images referenced from published markdown.
	app.Get("/media/:name", mediaH.Serve)

	// Syndication / SEO (static paths before /:slug).
	app.Get("/feed", syn.RSS)
	app.Get("/feed/atom", syn.Atom)
	app.Get("/feed.json", syn.JSONFeed)
	app.Get("/sitemap.xml", syn.Sitemap)
	app.Get("/robots.txt", syn.Robots)

	// Public auth routes (no auth required).
	app.Get("/admin/login", adminAuth.LoginPage)
	app.Post("/admin/login", adminAuth.LoginSubmit)
	app.Post("/admin/logout", adminAuth.Logout)

	// Authenticated admin article CRUD. The index is registered at the exact
	// static "/admin" path (not the group's trailing-slash "/") so it wins
	// over the public "/:slug" parameter route; sub-routes use the group.
	app.Get("/admin", adminAuth.RequireAuth, articleAdmin.List)
	admin := app.Group("/admin", adminAuth.RequireAuth)
	// Static sub-routes first so they win over the /:id parameter route
	// (Fiber radix matches by registration order at the same depth).
	admin.Get("/new", articleAdmin.NewForm)
	admin.Post("/new", articleAdmin.Create)
	admin.Get("/settings", settings.Form)
	admin.Post("/settings", settings.Save)
	admin.Get("/upload", uploads.Form)
	admin.Post("/upload", uploads.Submit)
	admin.Post("/media", mediaH.Upload)
	admin.Get("/tags", tags.List)
	admin.Post("/tags/rename", tags.Rename)
	admin.Post("/tags/merge", tags.Merge)
	admin.Get("/redirects", redirects.List)
	admin.Post("/redirects", redirects.Create)
	admin.Post("/redirects/delete", redirects.Delete)
	admin.Get("/:id/edit", articleAdmin.EditForm)
	admin.Post("/:id", articleAdmin.Update)
	admin.Post("/:id/delete", articleAdmin.Delete)

	// Public reader-facing pages. Static discovery routes must register
	// before /:slug so they are not captured as slugs.
	pub := handler.NewPublic(d.Store, d.Signer, d.Hasher, d.SiteDefaultPass, d.ContentDir, baseURL)
	app.Get("/", pub.Index)
	app.Get("/search", pub.Search)
	app.Get("/archive", pub.ArchiveIndex)
	app.Get("/archive/:year", pub.ArchiveYear)
	app.Get("/archive/:year/:month", pub.ArchiveMonth)
	app.Get("/tag/:tag", pub.Tag)
	app.Get("/preview/:token", pub.Preview)
	// Unlock / embed content before the wildcard asset route.
	app.Get("/:slug/unlock", pub.UnlockForm)
	app.Post("/:slug/unlock", pub.UnlockSubmit)
	app.Get("/:slug/~content", pub.UploadContent) // raw HTML for iframe (non-raw_mode shell)
	// Article entry + static assets for html_upload (relative css/slides/…).
	app.Get("/:slug", pub.Article)
	app.Get("/:slug/*", pub.UploadAsset)

	return app
}
