package sitebrand_test

import (
	"context"
	"testing"

	"github.com/sam33339999/wikibuild/internal/sitebrand"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	b := sitebrand.Load(context.Background(), inmem.New(), "MySite")
	require.Equal(t, "MySite", b.Name)
	require.NotEmpty(t, b.Tagline)
}

func TestLoad_FromSettings(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()
	require.NoError(t, repo.SetSetting(ctx, sitebrand.KeyName, "Sam Lab"))
	require.NoError(t, repo.SetSetting(ctx, sitebrand.KeyTagline, "Build in public"))
	require.NoError(t, repo.SetSetting(ctx, sitebrand.KeyAuthor, "Sam"))
	require.NoError(t, repo.SetSetting(ctx, sitebrand.KeyGitHub, "sam33339999"))

	b := sitebrand.Load(ctx, repo, "Fallback")
	require.Equal(t, "Sam Lab", b.Name)
	require.Equal(t, "Build in public", b.Tagline)
	require.Equal(t, "Sam", b.Author)
	require.Equal(t, "https://github.com/sam33339999", b.GitHubURL())
}

func TestXURL_Handle(t *testing.T) {
	b := sitebrand.Brand{X: "@hello"}
	require.Equal(t, "https://x.com/hello", b.XURL())
}
