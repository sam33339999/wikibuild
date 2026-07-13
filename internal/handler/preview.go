package handler

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/render"
	"github.com/sam33339999/wikibuild/internal/store"
	publicviews "github.com/sam33339999/wikibuild/views/public"
)

// Preview serves unlisted draft share links at /preview/:token.
func (h *Public) Preview(c fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.SendStatus(http.StatusNotFound)
	}
	a, err := h.repo.GetArticleByPreviewToken(c.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.SendStatus(http.StatusNotFound)
		}
		return err
	}
	// No comments on previews; no public backlinks either.
	emptyComments := publicviews.CommentConfig{}
	if a.Type == model.ArticleTypeHTMLUpload {
		data, err := readUploadFile(h.contentDir, a.Slug, a.Body)
		if err != nil {
			return err
		}
		if a.RawMode {
			return c.Type("html").Send(data)
		}
		return renderPage(c, a.Title+" (預覽)", publicviews.Article(a, string(data), nil, 0, nil, emptyComments))
	}
	html, toc := render.RenderWithTOC(a.Body)
	return renderPage(c, a.Title+" (預覽)", publicviews.Article(
		a, html, toc, render.ReadingTime(a.Body), nil, emptyComments))
}
