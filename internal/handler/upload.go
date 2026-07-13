package handler

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	adminviews "github.com/sam33339999/wikibuild/views/admin"
)

// safeSlugRe restricts slugs to filesystem-safe characters (they become a
// directory name under the content dir, so path separators and ".." are
// forbidden).
var safeSlugRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Upload handles HTML static-page uploads. A .zip (containing index.html and
// optional assets) is extracted to <contentDir>/<slug>/; a single .html is
// saved as index.html. The created article's Body records the entry file.
// contentDir is injected so tests use t.TempDir().
type Upload struct {
	repo       store.Repository
	contentDir string
}

// NewUpload builds an Upload handler.
func NewUpload(repo store.Repository, contentDir string) *Upload {
	return &Upload{repo: repo, contentDir: contentDir}
}

// Form renders the upload page.
func (h *Upload) Form(c fiber.Ctx) error {
	return renderPage(c, "上傳 HTML", adminviews.Upload(csrf.TokenFromContext(c)))
}

// Submit processes the upload: validates, extracts the file, and creates an
// html_upload article. On any failure the extracted directory is removed so
// the filesystem and DB never diverge.
func (h *Upload) Submit(c fiber.Ctx) error {
	slug := strings.Clone(strings.TrimSpace(c.FormValue("slug")))
	if !safeSlugRe.MatchString(slug) {
		return c.Status(http.StatusBadRequest).SendString("invalid slug (use letters, digits, _ or -)")
	}
	title := strings.Clone(strings.TrimSpace(c.FormValue("title")))
	if title == "" {
		return c.Status(http.StatusBadRequest).SendString("title is required")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(http.StatusBadRequest).SendString("file is required")
	}
	src, err := fh.Open()
	if err != nil {
		return err
	}
	data, err := io.ReadAll(src)
	_ = src.Close()
	if err != nil {
		return err
	}

	target := filepath.Join(h.contentDir, slug)
	// Replace any previous extraction for this slug so re-uploads are clean.
	_ = os.RemoveAll(target)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	entry := "index.html"
	lower := strings.ToLower(fh.Filename)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		if err := extractZip(data, target); err != nil {
			_ = os.RemoveAll(target)
			return c.Status(http.StatusBadRequest).SendString("invalid zip: " + err.Error())
		}
		found, err := findHTMLEntry(target)
		if err != nil {
			_ = os.RemoveAll(target)
			return c.Status(http.StatusBadRequest).SendString(err.Error())
		}
		entry = found
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		if err := os.WriteFile(filepath.Join(target, "index.html"), data, 0o644); err != nil {
			_ = os.RemoveAll(target)
			return err
		}
	default:
		_ = os.RemoveAll(target)
		return c.Status(http.StatusBadRequest).SendString("file must be .zip or .html")
	}

	a := model.Article{
		Slug:       slug,
		Title:      title,
		Type:       model.ArticleTypeHTMLUpload,
		Status:     model.Status(strings.Clone(c.FormValue("status"))),
		Visibility: model.Visibility(strings.Clone(c.FormValue("visibility"))),
		Body:       entry,
		RawMode:    c.FormValue("raw_mode") == "on",
		Tags:       []string{},
	}
	if _, err := h.repo.CreateArticle(c.Context(), a); err != nil {
		_ = os.RemoveAll(target) // roll back the extraction
		switch {
		case errors.Is(err, store.ErrDuplicateSlug):
			return c.Status(http.StatusConflict).SendString("slug already exists")
		case errors.Is(err, store.ErrEmptySlug):
			return c.SendStatus(http.StatusBadRequest)
		default:
			return err
		}
	}
	return c.Redirect().To("/admin")
}

// extractZip writes a zip's entries under target, rejecting zip-slip, skipping
// macOS junk (__MACOSX / ._* files), and stripping a single shared top-level
// folder so index.html lands at the slug root when the archive is
// "folder/index.html" rather than "index.html".
func extractZip(data []byte, target string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	// Collect usable entry names and detect a single root folder prefix.
	var names []string
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if shouldSkipZipEntry(name) {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return errors.New("zip has no usable files (empty or only __MACOSX junk)")
	}
	prefix := sharedRootPrefix(names)

	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if shouldSkipZipEntry(name) {
			continue
		}
		rel := name
		if prefix != "" {
			rel = strings.TrimPrefix(name, prefix)
			if rel == "" {
				continue // the root folder entry itself
			}
		}
		path := filepath.Join(target, filepath.FromSlash(rel))
		if !within(target, path) {
			return errors.New("zip entry escapes target directory: " + f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := copyZipEntry(f, path); err != nil {
			return err
		}
	}
	return nil
}

// shouldSkipZipEntry drops macOS resource forks and empty names.
func shouldSkipZipEntry(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." {
		return true
	}
	// __MACOSX/… and AppleDouble "._file"
	base := filepath.Base(name)
	if strings.HasPrefix(name, "__MACOSX/") || strings.Contains(name, "/__MACOSX/") {
		return true
	}
	if strings.HasPrefix(base, "._") {
		return true
	}
	return false
}

// sharedRootPrefix returns "folder/" when every path is under that single
// top-level directory; otherwise "".
func sharedRootPrefix(names []string) string {
	var prefix string
	for _, n := range names {
		n = strings.TrimPrefix(n, "./")
		parts := strings.SplitN(n, "/", 2)
		if len(parts) < 2 {
			// File at zip root → no shared folder to strip.
			return ""
		}
		root := parts[0] + "/"
		if prefix == "" {
			prefix = root
			continue
		}
		if prefix != root {
			return ""
		}
	}
	return prefix
}

// findHTMLEntry returns a path relative to root for the page entry:
// prefers index.html / index.htm (any depth, shallowest first), else the
// shallowest *.html file.
func findHTMLEntry(root string) (string, error) {
	var candidates []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "__MACOSX" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(info.Name())
		if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			candidates = append(candidates, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", errors.New("zip has no .html entry file")
	}
	// Prefer index.html (any depth), then shallowest path.
	best := candidates[0]
	bestScore := entryScore(best)
	for _, c := range candidates[1:] {
		if s := entryScore(c); s < bestScore {
			best, bestScore = c, s
		}
	}
	return best, nil
}

// entryScore: lower is better. index.html preferred; fewer path segments preferred.
func entryScore(rel string) int {
	score := strings.Count(rel, "/") * 10
	base := strings.ToLower(filepath.Base(rel))
	if base == "index.html" || base == "index.htm" {
		return score
	}
	return score + 100
}

func copyZipEntry(f *zip.File, path string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// within reports whether path is inside base (after cleaning), guarding
// against "../" escapes.
func within(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
