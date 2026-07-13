package handler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/stretchr/testify/require"
)

func TestPublic_Preview_RendersDraft(t *testing.T) {
	app, repo, _, _, _ := publicApp(t)
	_, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "secret-draft", Title: "Secret Draft",
		Body: "# Hidden\n\nOnly via token.",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft,
		Visibility: model.VisibilityPrivate, PreviewToken: "preview-xyz",
	})
	require.NoError(t, err)

	// Normal slug is 404 (draft + private).
	resp, _ := app.Test(httptest.NewRequest(http.MethodGet, "/secret-draft", nil))
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Preview token works.
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/preview/preview-xyz", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "Secret Draft")
	require.Contains(t, string(body), "Only via token")
}

func TestPublic_Preview_UnknownToken404(t *testing.T) {
	app, _, _, _, _ := publicApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/preview/no-such-token", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
