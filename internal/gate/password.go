package gate

import (
	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/model"
)

// MatchPassword checks a reader-supplied password against an article's
// protection, following the README priority: an article-specific password
// (bcrypt) takes precedence; otherwise the site-wide default is compared
// directly. Returns false if neither is set or the input is empty.
func MatchPassword(a model.Article, input, siteDefault string, h auth.PasswordHasher) bool {
	if input == "" {
		return false
	}
	if a.Password != "" {
		return h.Compare(a.Password, input) == nil
	}
	return input == siteDefault && siteDefault != ""
}
