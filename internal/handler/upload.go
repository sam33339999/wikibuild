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
		return c.SendStatus(http.StatusBadRequest)
	}
	title := strings.Clone(strings.TrimSpace(c.FormValue("title")))
	if title == "" {
		return c.SendStatus(http.StatusBadRequest)
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
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
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	lower := strings.ToLower(fh.Filename)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		if err := extractZip(data, target); err != nil {
			_ = os.RemoveAll(target)
			return c.SendStatus(http.StatusBadRequest)
		}
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		if err := os.WriteFile(filepath.Join(target, "index.html"), data, 0o644); err != nil {
			_ = os.RemoveAll(target)
			return err
		}
	default:
		_ = os.RemoveAll(target)
		return c.SendStatus(http.StatusBadRequest)
	}

	a := model.Article{
		Slug:       slug,
		Title:      title,
		Type:       model.ArticleTypeHTMLUpload,
		Status:     model.Status(strings.Clone(c.FormValue("status"))),
		Visibility: model.Visibility(strings.Clone(c.FormValue("visibility"))),
		Body:       "index.html",
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

// extractZip writes a zip's entries under target, rejecting any entry whose
// path would escape target (zip-slip). Directories are created; files are
// written with 0o644.
func extractZip(data []byte, target string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		path := filepath.Join(target, f.Name)
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
