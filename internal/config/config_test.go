package config_test

import (
	"errors"
	"testing"

	"github.com/sam33339999/wikibuild/internal/config"
	"github.com/stretchr/testify/require"
)

// staticLookup builds a LookupFunc from a map for tests.
func staticLookup(env map[string]string) config.LookupFunc {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":                     "postgres://u:p@localhost:5432/wikibuild",
		"WIKIBUILD_ADMIN_USER":             "admin",
		"WIKIBUILD_ADMIN_PASS":             "s3cret",
		"WIKIBUILD_SESSION_SECRET":         "0123456789abcdef0123456789abcdef",
		"WIKIBUILD_PORT":                   "8080",
		"WIKIBUILD_CONTENT_DIR":            "./content/uploads",
		"WIKIBUILD_DEFAULT_PROTECTED_PASS": "sitepass",
	}
}

func TestLoad_OK(t *testing.T) {
	cfg, err := config.Load(staticLookup(validEnv()))
	require.NoError(t, err)
	require.Equal(t, "postgres://u:p@localhost:5432/wikibuild", cfg.DatabaseURL)
	require.Equal(t, "admin", cfg.AdminUser)
	require.Equal(t, "s3cret", cfg.AdminPass)
	require.Equal(t, "0123456789abcdef0123456789abcdef", cfg.SessionSecret)
	require.Equal(t, 8080, cfg.Port)
	require.Equal(t, "./content/uploads", cfg.ContentDir)
	require.Equal(t, "sitepass", cfg.DefaultProtectedPass)
}

func TestLoad_AppliesDefaultsWhenOptionalUnset(t *testing.T) {
	env := validEnv()
	delete(env, "WIKIBUILD_HOST")
	delete(env, "WIKIBUILD_PORT")
	delete(env, "WIKIBUILD_CONTENT_DIR")
	delete(env, "WIKIBUILD_DEFAULT_PROTECTED_PASS")

	cfg, err := config.Load(staticLookup(env))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", cfg.Host, "default host is localhost")
	require.Equal(t, 8080, cfg.Port, "default port")
	require.Equal(t, "./content/uploads", cfg.ContentDir, "default content dir")
	require.Empty(t, cfg.DefaultProtectedPass, "no default protected pass")
}

func TestLoad_HostOverride(t *testing.T) {
	env := validEnv()
	env["WIKIBUILD_HOST"] = "0.0.0.0"
	cfg, err := config.Load(staticLookup(env))
	require.NoError(t, err)
	require.Equal(t, "0.0.0.0", cfg.Host)
}

func TestLoad_MissingRequiredVars(t *testing.T) {
	for _, key := range []string{
		"DATABASE_URL", "WIKIBUILD_ADMIN_USER",
		"WIKIBUILD_ADMIN_PASS", "WIKIBUILD_SESSION_SECRET",
	} {
		env := validEnv()
		delete(env, key)
		t.Run(key, func(t *testing.T) {
			_, err := config.Load(staticLookup(env))
			require.Error(t, err, "missing %s must error", key)
			require.ErrorIs(t, err, config.ErrMissingEnv, "%s missing should be ErrMissingEnv", key)
			require.Contains(t, err.Error(), key, "error should name the missing var")
		})
	}
}

func TestLoad_SessionSecretTooShort(t *testing.T) {
	env := validEnv()
	env["WIKIBUILD_SESSION_SECRET"] = "short"
	_, err := config.Load(staticLookup(env))
	require.Error(t, err)
	require.ErrorIs(t, err, config.ErrInvalidConfig)
}

func TestLoad_InvalidPort(t *testing.T) {
	env := validEnv()
	env["WIKIBUILD_PORT"] = "not-a-number"
	_, err := config.Load(staticLookup(env))
	require.Error(t, err)
	require.ErrorIs(t, err, config.ErrInvalidConfig)
}

func TestLoad_OutOfRangePort(t *testing.T) {
	env := validEnv()
	env["WIKIBUILD_PORT"] = "99999"
	_, err := config.Load(staticLookup(env))
	require.Error(t, err)
	require.ErrorIs(t, err, config.ErrInvalidConfig)
}

func TestLoad_AllMissingJoinsErrors(t *testing.T) {
	_, err := config.Load(staticLookup(map[string]string{}))
	require.Error(t, err)
	var me interface{ Unwrap() []error }
	require.True(t, errors.As(err, &me), "should report all missing via errors.Join")
	require.NotEmpty(t, me.Unwrap())
}
