package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

	related      []llm.RelatedSuggestion
	relatedErr   error
	relatedCalls int
	lastSel      string
	lastCatN     int
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

func (m *mockSEOClient) SuggestRelated(ctx context.Context, selection string, catalog []llm.CatalogEntry) ([]llm.RelatedSuggestion, error) {
	m.relatedCalls++
	m.lastSel = selection
	m.lastCatN = len(catalog)
	if m.relatedErr != nil {
		return nil, m.relatedErr
	}
	return m.related, nil
}

func (m *mockSEOClient) StreamChat(ctx context.Context, messages []llm.Message, onDelta func(string) error) error {
	return llm.ErrNotConfigured
}

func (m *mockSEOClient) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (llm.ChatResult, error) {
	return llm.ChatResult{}, llm.ErrNotConfigured
}

func aiseoApp(t *testing.T, client llm.Client, contentDir, mediaDir string) (*fiber.App, store.Repository, *handler.AISEO) {
	t.Helper()
	repo := inmem.New()
	h := handler.NewAISEO(repo, client, nil, contentDir, mediaDir, "Test Site")
	app := fiber.New()
	app.Post("/admin/ai/seo", h.Generate)
	app.Post("/admin/:id/ai/seo", h.GenerateForArticle)
	app.Post("/admin/ai/related", h.SuggestRelated)
	app.Post("/admin/:id/ai/og", h.GenerateOG)
	return app, repo, h
}

func TestAISEO_Generate_NotConfigured(t *testing.T) {
	client := &mockSEOClient{enabled: false}
	app, _, _ := aiseoApp(t, client, "", "")

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
	app, _, _ := aiseoApp(t, client, "", "")

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
	app, _, _ := aiseoApp(t, client, "", "")

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
	app, repo, _ := aiseoApp(t, client, "", "")
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "p", Title: "Stored Title", Body: "Stored body content",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)

	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/admin/"+strconv.FormatInt(a.ID, 10)+"/ai/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "Stored Title", client.lastT)
	require.Equal(t, "Stored body content", client.lastB)
}

func TestAISEO_GenerateForArticle_HTMLUploadExtractsText(t *testing.T) {
	client := &mockSEOClient{
		enabled: true,
		result:  llm.SEOResult{Outline: []string{"o"}, MetaDescription: "m", Summary: "s"},
	}
	dir := t.TempDir()
	slugDir := filepath.Join(dir, "slides")
	require.NoError(t, os.MkdirAll(slugDir, 0o755))
	html := `<html><body><h1>Deck Title</h1><p>Important content for SEO.</p><script>bad()</script></body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(slugDir, "index.html"), []byte(html), 0o644))

	app, repo, _ := aiseoApp(t, client, dir, "")
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "slides", Title: "My Deck", Body: "index.html",
		Type: model.ArticleTypeHTMLUpload, Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/admin/"+strconv.FormatInt(a.ID, 10)+"/ai/seo", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "My Deck", client.lastT)
	require.Contains(t, client.lastB, "Important content")
	require.NotContains(t, client.lastB, "bad()")
	require.NotContains(t, client.lastB, "<h1")
}

func TestAISEO_GenerateForArticle_NotFound(t *testing.T) {
	client := &mockSEOClient{enabled: true}
	app, _, _ := aiseoApp(t, client, "", "")
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
	app, repo, _ := aiseoApp(t, client, "", "")
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
	h := handler.NewAISEO(repo, client, &handler.AISEOLimit{Max: 2, Window: time.Minute}, "", "", "T")
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

func TestAISEO_SuggestRelated_Success(t *testing.T) {
	client := &mockSEOClient{
		enabled: true,
		related: []llm.RelatedSuggestion{
			{Slug: "go-tips", Title: "Go Tips", Reason: "same stack"},
			{Slug: "ghost", Title: "Ghost", Reason: "hallucinated"},
		},
	}
	app, repo, _ := aiseoApp(t, client, "", "")
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "go-tips", Title: "Go Tips", Body: "x", Type: model.ArticleTypeMarkdown,
		Status: model.StatusPublished, Visibility: model.VisibilityPublic, Summary: "go",
	})
	_, _ = repo.CreateArticle(context.Background(), model.Article{
		Slug: "other", Title: "Other", Body: "y", Type: model.ArticleTypeMarkdown,
		Status: model.StatusDraft, Visibility: model.VisibilityPrivate,
	})

	form := url.Values{"selection": {"talking about golang interfaces"}, "exclude_id": {"0"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/related", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	sugs, ok := got["suggestions"].([]any)
	require.True(t, ok)
	require.Len(t, sugs, 1, "hallucinated slug filtered out")
	first := sugs[0].(map[string]any)
	require.Equal(t, "go-tips", first["slug"])
	require.Equal(t, "same stack", first["reason"])
	require.Equal(t, 1, client.relatedCalls)
	require.Contains(t, client.lastSel, "golang")
	require.GreaterOrEqual(t, client.lastCatN, 2)
}

func TestAISEO_SuggestRelated_EmptySelection(t *testing.T) {
	client := &mockSEOClient{enabled: true}
	app, _, _ := aiseoApp(t, client, "", "")
	form := url.Values{"selection": {"  "}}
	req := httptest.NewRequest(http.MethodPost, "/admin/ai/related", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, 0, client.relatedCalls)
}

func TestAISEO_GenerateOG_WritesMediaPNG(t *testing.T) {
	client := &mockSEOClient{enabled: false} // OG does not need LLM
	mediaDir := t.TempDir()
	app, repo, _ := aiseoApp(t, client, "", mediaDir)
	a, err := repo.CreateArticle(context.Background(), model.Article{
		Slug: "hello", Title: "Hello OG", Body: "x",
		Type: model.ArticleTypeMarkdown, Status: model.StatusDraft, Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)

	form := url.Values{"title": {"Custom Title"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/"+strconv.FormatInt(a.ID, 10)+"/ai/og", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	urlStr, _ := got["url"].(string)
	require.True(t, strings.HasPrefix(urlStr, "/media/"))
	require.True(t, strings.HasSuffix(urlStr, ".png"))
	name := got["name"].(string)
	data, err := os.ReadFile(filepath.Join(mediaDir, name))
	require.NoError(t, err)
	require.True(t, len(data) > 100)
	require.Equal(t, byte(0x89), data[0])
	// Does not mutate article
	gotA, _ := repo.GetArticle(context.Background(), a.ID)
	require.Empty(t, gotA.OGImageURL)
}
