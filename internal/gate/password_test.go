package gate_test

import (
	"testing"

	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/gate"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/stretchr/testify/require"
)

// stubHasher lets tests control hash/compare without bcrypt's cost. A stored
// hash of "H:"+plaintext compares equal only to the matching plaintext.
type stubHasher struct{}

func (stubHasher) Hash(p string) (string, error) { return "H:" + p, nil }
func (stubHasher) Compare(h, p string) error {
	if h == "H:"+p {
		return nil
	}
	return auth.ErrPasswordMismatch
}

func TestMatchPassword_ArticlePassword_Correct(t *testing.T) {
	a := model.Article{Password: "H:s3cret"}
	require.True(t, gate.MatchPassword(a, "s3cret", "default", stubHasher{}))
}

func TestMatchPassword_ArticlePassword_Wrong(t *testing.T) {
	a := model.Article{Password: "H:s3cret"}
	require.False(t, gate.MatchPassword(a, "wrong", "default", stubHasher{}))
	// Site default must NOT be used when an article password is set.
	require.False(t, gate.MatchPassword(a, "default", "default", stubHasher{}))
}

func TestMatchPassword_FallsBackToSiteDefault(t *testing.T) {
	a := model.Article{Password: ""}
	require.True(t, gate.MatchPassword(a, "sitedefault", "sitedefault", stubHasher{}))
}

func TestMatchPassword_SiteDefault_Wrong(t *testing.T) {
	a := model.Article{Password: ""}
	require.False(t, gate.MatchPassword(a, "wrong", "sitedefault", stubHasher{}))
}

func TestMatchPassword_NoArticlePasswordNoSiteDefault(t *testing.T) {
	a := model.Article{Password: ""}
	require.False(t, gate.MatchPassword(a, "anything", "", stubHasher{}))
}

func TestMatchPassword_EmptyInputNeverMatches(t *testing.T) {
	a := model.Article{Password: ""}
	require.False(t, gate.MatchPassword(a, "", "sitedefault", stubHasher{}))
}
