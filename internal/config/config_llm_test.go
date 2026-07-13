package config_test

import (
	"testing"

	"github.com/sam33339999/wikibuild/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoad_LLMOptionalDefaultsEmpty(t *testing.T) {
	cfg, err := config.Load(staticLookup(validEnv()))
	require.NoError(t, err)
	require.Empty(t, cfg.LLMAPIKey)
	require.Empty(t, cfg.LLMBaseURL)
	require.Empty(t, cfg.LLMModel)
	require.False(t, cfg.LLMEnabled())
}

func TestLoad_LLMFields(t *testing.T) {
	env := validEnv()
	env["WIKIBUILD_LLM_BASE_URL"] = "https://api.x.ai/v1"
	env["WIKIBUILD_LLM_API_KEY"] = "sk-test"
	env["WIKIBUILD_LLM_MODEL"] = "grok-4.5"
	cfg, err := config.Load(staticLookup(env))
	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1", cfg.LLMBaseURL)
	require.Equal(t, "sk-test", cfg.LLMAPIKey)
	require.Equal(t, "grok-4.5", cfg.LLMModel)
	require.True(t, cfg.LLMEnabled())
}

func TestLoad_MCPTokenOptional(t *testing.T) {
	cfg, err := config.Load(staticLookup(validEnv()))
	require.NoError(t, err)
	require.Empty(t, cfg.MCPToken)

	env := validEnv()
	env["WIKIBUILD_MCP_TOKEN"] = "secret-token"
	cfg, err = config.Load(staticLookup(env))
	require.NoError(t, err)
	require.Equal(t, "secret-token", cfg.MCPToken)
}

func TestLoad_LLMEnabled_RequiresKeyAndBaseAndModel(t *testing.T) {
	env := validEnv()
	env["WIKIBUILD_LLM_API_KEY"] = "sk"
	// missing base + model
	cfg, err := config.Load(staticLookup(env))
	require.NoError(t, err)
	require.False(t, cfg.LLMEnabled())

	env["WIKIBUILD_LLM_BASE_URL"] = "https://api.example/v1"
	cfg, err = config.Load(staticLookup(env))
	require.NoError(t, err)
	require.False(t, cfg.LLMEnabled(), "still needs model")

	env["WIKIBUILD_LLM_MODEL"] = "m"
	cfg, err = config.Load(staticLookup(env))
	require.NoError(t, err)
	require.True(t, cfg.LLMEnabled())
}
