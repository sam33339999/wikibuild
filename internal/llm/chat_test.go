package llm_test

import (
	"strings"
	"testing"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestBuildChatMessages_SystemAndUser(t *testing.T) {
	msgs, err := llm.BuildChatMessages("be brief", nil, "hello")
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, "system", msgs[0].Role)
	require.Equal(t, "be brief", msgs[0].Content)
	require.Equal(t, "user", msgs[1].Role)
	require.Equal(t, "hello", msgs[1].Content)
}

func TestBuildChatMessages_WithHistory(t *testing.T) {
	hist := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello!"},
	}
	msgs, err := llm.BuildChatMessages("", hist, "how are you?")
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "assistant", msgs[1].Role)
	require.Equal(t, "how are you?", msgs[2].Content)
}

func TestBuildChatMessages_EmptyUser(t *testing.T) {
	_, err := llm.BuildChatMessages("", nil, "  ")
	require.ErrorIs(t, err, llm.ErrEmptyBody)
}

func TestBuildChatMessages_RejectsBadHistoryRole(t *testing.T) {
	_, err := llm.BuildChatMessages("", []llm.Message{{Role: "tool", Content: "x"}}, "hi")
	require.ErrorIs(t, err, llm.ErrInvalidChat)
}

func TestBuildChatMessages_ClipsLongUser(t *testing.T) {
	long := strings.Repeat("字", llm.MaxBodyBytes/3+100)
	msgs, err := llm.BuildChatMessages("", nil, long)
	require.NoError(t, err)
	require.LessOrEqual(t, len(msgs[0].Content), llm.MaxBodyBytes)
}

func TestBuildChatMessages_TrimsHistoryCap(t *testing.T) {
	// More than MaxChatHistory turns → keep the latest pairs.
	var hist []llm.Message
	for i := 0; i < llm.MaxChatHistory+10; i++ {
		hist = append(hist, llm.Message{Role: "user", Content: "u"})
		hist = append(hist, llm.Message{Role: "assistant", Content: "a"})
	}
	msgs, err := llm.BuildChatMessages("sys", hist, "final")
	require.NoError(t, err)
	// system + capped history + final user
	require.LessOrEqual(t, len(msgs), 1+llm.MaxChatHistory+1)
	require.Equal(t, "final", msgs[len(msgs)-1].Content)
	require.Equal(t, "system", msgs[0].Role)
}
