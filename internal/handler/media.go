package handler

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/media"
)

// Media handles simple image uploads for the markdown editor (paste / drag-
// drop). Files live under mediaDir and are served at GET /media/:name.
// mediaDir is injected so tests use t.TempDir().
type Media struct {
	mediaDir string
}

// NewMedia builds a Media handler writing into mediaDir.
func NewMedia(mediaDir string) *Media {
	return &Media{mediaDir: mediaDir}
}

// Upload accepts a multipart "file" field, validates it as an image, saves
// it, and returns JSON {"url","name"} for the editor to insert as markdown.
func (h *Media) Upload(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "missing file"})
	}
	// Bound read to MaxBytes+1 so we can distinguish "too large" from OK.
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, int64(media.MaxBytes)+1))
	if err != nil {
		return err
	}
	got, err := media.Save(h.mediaDir, data)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrEmpty):
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "empty file"})
		case errors.Is(err, media.ErrTooLarge):
			return c.Status(http.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": "file too large"})
		case errors.Is(err, media.ErrUnsupportedType):
			return c.Status(http.StatusUnsupportedMediaType).JSON(fiber.Map{"error": "unsupported image type"})
		default:
			return err
		}
	}
	return c.JSON(fiber.Map{"url": got.URL, "name": got.Name})
}

// Serve streams a saved image by basename. Public so published articles can
// reference /media/... without an admin session.
func (h *Media) Serve(c fiber.Ctx) error {
	name := c.Params("name")
	// Fiber may leave URL-encoded values; basename already, but be strict.
	name = filepath.Base(name)
	if !media.SafeName(name) {
		return c.SendStatus(http.StatusNotFound)
	}
	path := filepath.Join(h.mediaDir, name)
	c.Set("Content-Type", media.ContentType(name))
	// Cache images for a day; names are content-unique random ids.
	c.Set("Cache-Control", "public, max-age=86400")
	// SendFile reads and closes the file itself; Open+SendStream races with
	// the handler's defer Close under app.Test.
	if err := c.SendFile(path); err != nil {
		return c.SendStatus(http.StatusNotFound)
	}
	return nil
}
