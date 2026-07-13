// Package handler contains Fiber HTTP handlers for WikiBuilder. Handlers
// depend only on store.Repository and the auth primitives, so they are unit
// tested against the inmem store with a fake hasher (no bcrypt cost, no DB).
package handler

import (
	"bytes"
	"errors"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// Cookie / route constants.
const (
	sessionCookie = "wikibuild_admin"
	sessionTTL    = 24 * time.Hour
)

// ctxKey is an unexported type so Locals keys don't collide.
type ctxKey int

const adminKey ctxKey = 0

// AdminAuthDeps bundles the injectable collaborators for admin auth.
type AdminAuthDeps struct {
	Store   store.Repository
	Hasher  auth.PasswordHasher
	Signer  *auth.Signer
	Limiter *auth.LoginLimiter
	// Clock is optional; if nil, auth.Real is used for cookie expiry math.
	Clock clock.Clock
}

// AdminAuth handles login, logout and the authenticated dashboard for M0.
type AdminAuth struct {
	deps AdminAuthDeps
}

// NewAdminAuth builds an AdminAuth handler with the given dependencies.
func NewAdminAuth(d AdminAuthDeps) *AdminAuth {
	if d.Clock == nil {
		d.Clock = clock.Real{}
	}
	return &AdminAuth{deps: d}
}

// LoginPage renders the login form via templ. The CSRF token from the
// middleware is embedded as a hidden field.
func (h *AdminAuth) LoginPage(c fiber.Ctx) error {
	tok := csrf.TokenFromContext(c)
	var buf bytes.Buffer
	if err := adminviews.Login(tok).Render(c.Context(), &buf); err != nil {
		return err
	}
	return c.Type("html").Send(buf.Bytes())
}

// LoginSubmit authenticates an admin and sets a signed session cookie.
// It is brute-force protected by the LoginLimiter keyed on client IP.
func (h *AdminAuth) LoginSubmit(c fiber.Ctx) error {
	ip := c.IP()
	if h.deps.Limiter.IsLocked(ip) {
		return c.Status(http.StatusTooManyRequests).SendString("too many attempts, try later")
	}

	username := c.FormValue("username")
	password := c.FormValue("password")

	user, err := h.deps.Store.GetUserByUsername(c.Context(), username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.deps.Limiter.RegisterFailure(ip)
			return c.Status(http.StatusUnauthorized).SendString("invalid credentials")
		}
		return err
	}

	if err := h.deps.Hasher.Compare(user.PasswordHash, password); err != nil {
		h.deps.Limiter.RegisterFailure(ip)
		return c.Status(http.StatusUnauthorized).SendString("invalid credentials")
	}

	h.deps.Limiter.RegisterSuccess(ip)
	tok, err := h.deps.Signer.Sign(user.Username, sessionTTL)
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Expires:  h.deps.Clock.Now().Add(sessionTTL),
	})
	return c.Redirect().To("/admin")
}

// Logout clears the session cookie and redirects to the login page.
func (h *AdminAuth) Logout(c fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   -1,
	})
	return c.Redirect().To("/admin/login")
}

// RequireAuth is middleware that verifies the session cookie. On success it
// stores the username in Locals and continues; otherwise it redirects to the
// login page (302).
func (h *AdminAuth) RequireAuth(c fiber.Ctx) error {
	tok := c.Cookies(sessionCookie)
	if tok == "" {
		return c.Redirect().Status(http.StatusFound).To("/admin/login")
	}
	username, err := h.deps.Signer.Verify(tok)
	if err != nil {
		return c.Redirect().Status(http.StatusFound).To("/admin/login")
	}
	c.Locals(adminKey, username)
	return c.Next()
}

// AdminUsername reads the authenticated admin's username from request Locals
// (set by RequireAuth). Returns "" when not authenticated.
func AdminUsername(c fiber.Ctx) string {
	username, _ := c.Locals(adminKey).(string)
	return username
}
