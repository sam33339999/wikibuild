package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/handler"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

// mockSEOClient is a test double for llm.Client.
type mockSEOClient struct {
	enabled bool
	result  llm.SEOResult
	err     error
	calls   int
	lastT   string
	lastB   string
}

func (m *mockSEOClient) Enabled() bool { return m.enabled }

func (m *mockSEOClient) GenerateSEO(ctx context.Context, title, body string) (llm.SEOResult, error) {
	m.calls++
	m.lastT, m.lastB = title, body
	if m.err != nil {
		return llm.SEOResult{}, m.err
	}
	return m.result, nil
}

func aiseoApp(t *testing.T, client llm.Client) (*fiber.App, store.Repository, *handler.AISEO) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewAISEO(repo, client, nil)
	app := fiber.New()
	app.Post("/admin/ai/seo", h.Generate)
	app.Post("/admin/:id/ai/seo", h.GenerateForArticle)
	return app, repo, h
}

func TestAISEO_Generate_NotConfigured(t *testing.T) {
	client := &mockSEOClient{enabled: false}
	app, _, _ := aiseoApp(t, client)

	form := url.Values{"title": {"T"}, "body": {"# body"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "not configured")
	require.Equal(t, 0, client.calls)
}

func TestAISEO_Generate_SuccessJSON(t *testing.T) {
	client := &mockSEOClient{
		enabled: true,
		result: llm.SEOResult{
			Outline:         []string{"A", "B"},
			MetaDescription: "Meta",
			Summary:         "Sum",
		},
	}
	app, _, _ := aiseoApp(t, client)

	form := url.Values{"title": {"Hello"}, "body": {"# Hello\n\nWorld"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "Meta", got["meta_description"])
	require.Equal(t, "Sum", got["summary"])
	outline, ok := got["outline"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"A", "B"}, outline)
	require.Equal(t, 1, client.calls)
	require.Equal(t, "Hello", client.lastT)
	require.Contains(t, client.lastB, "World")
}

func TestAISEO_Generate_EmptyBody(t *testing.T) {
	client := &mockSEOClient{enabled: true}
	app, _, _ := aiseoApp(t, client)

	form := url.Values{"title": {"T"}, "body": {"  "}}
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, 0, client.calls)
}

func TestAISEO_GenerateForArticle_UsesStoredBody(t *testing.T) {
	client := &mockSEOClient{
		enabled: true,
		result:  llm.SEOResult{Outline: []string{"x"}, MetaDescription: "m", Summary: "s"},
	}
	app, repo, _ := aiseoApp(t, client)
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "p", Title: "Stored Title", Body: "Stored body content",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)

	// No body in form → load from article.
	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/admin/"+strconv.FormatInt(a.ID, 10)+"/ai/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "Stored Title", client.lastT)
	require.Equal(t, "Stored body content", client.lastB)
}

func TestAISEO_GenerateForArticle_NotFound(t *testing.T) {
	client := &mockSEOClient{enabled: true}
	app, _, _ := aiseoApp(t, client)
	req := httptest.NewRequest(http.MethodPost, "/admin/999/ai/seo", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAISEO_Generate_DoesNotPersistArticle(t *testing.T) {
	client := &mockSEOClient{
		enabled: true,
		result:  llm.SEOResult{Outline: []string{"o"}, MetaDescription: "new meta", Summary: "new sum"},
	}
	app, repo, _ := aiseoApp(t, client)
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "keep", Title: "T", Body: "body",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
		MetaDescription: "old meta",
	})
	require.NoError(t, err)

	form := url.Values{"title": {"T"}, "body": {"body"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := repo.GetArticle(context.Background(), a.ID)
	require.NoError(t, err)
	require.Equal(t, "old meta", got.MetaDescription, "AI generate must not write to DB")
	require.Equal(t, model.StatusDraft, got.Status)
}

func TestAISEO_Generate_RateLimited(t *testing.T) {
	client := &mockSEOClient{
		enabled: true,
		result:  llm.SEOResult{Outline: []string{"o"}, MetaDescription: "m", Summary: "s"},
	}
	repo := inmem.New()
	// Very tight limit for the test.
	h := handler.NewAISEO(repo, client, &handler.AISEOLimit{Max: 2, Window: time.Minute})
	app := fiber.New()
	app.Post("/admin/ai/seo", h.Generate)

	post := func() *http.Response {
		form := url.Values{"title": {"T"}, "body": {"body text"}}
		req := httptest.NewRequest(http.MethodPost, "/admin/ai/seo", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := app.Test(req)
		require.NoError(t, err)
		return resp
	}
	require.Equal(t, http.StatusOK, post().StatusCode)
	require.Equal(t, http.StatusOK, post().StatusCode)
	require.Equal(t, http.StatusTooManyRequests, post().StatusCode)
	require.Equal(t, 2, client.calls)
}
