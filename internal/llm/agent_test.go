package llm_test

import (
	"context"
	"testing"

	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/stretchr/testify/require"
)

type mockAgentClient struct {
	enabled bool
	// responses[i] is the i-th Chat call result
	responses []llm.ChatResult
	calls     int
	lastTools int
}

func (m *mockAgentClient) Enabled() bool { return m.enabled }

func (m *mockAgentClient) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (llm.ChatResult, error) {
	m.calls++
	m.lastTools = len(tools)
	if m.calls-1 >= len(m.responses) {
		return llm.ChatResult{Content: "fallback"}, nil
	}
	return m.responses[m.calls-1], nil
}

type mockExec struct {
	calls []string
}

func (m *mockExec) Execute(ctx context.Context, name, argumentsJSON string) (string, error) {
	m.calls = append(m.calls, name)
	return `{"ok":true,"name":"` + name + `"}`, nil
}

func TestRunAgent_NoToolsFinalAnswer(t *testing.T) {
	client := &mockAgentClient{
		enabled:   true,
		responses: []llm.ChatResult{{Content: "Hello there"}},
	}
	var deltas string
	err := llm.RunAgent(context.Background(), client, &mockExec{}, []llm.Message{
		{Role: "user", Content: "hi"},
	}, llm.ArticleToolDefs(), func(ev llm.AgentEvent) error {
		if ev.Type == "delta" {
			deltas += ev.Delta
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "Hello there", deltas)
	require.Equal(t, 1, client.calls)
	require.Greater(t, client.lastTools, 0)
}

func TestRunAgent_OneToolRound(t *testing.T) {
	client := &mockAgentClient{
		enabled: true,
		responses: []llm.ChatResult{
			{
				ToolCalls: []llm.ToolCall{{ID: "1", Name: "list_articles", Arguments: `{"q":"go"}`}},
			},
			{Content: "Found 1 article about go."},
		},
	}
	exec := &mockExec{}
	var types []string
	var deltas string
	err := llm.RunAgent(context.Background(), client, exec, []llm.Message{
		{Role: "user", Content: "list go posts"},
	}, llm.ArticleToolDefs(), func(ev llm.AgentEvent) error {
		types = append(types, ev.Type)
		if ev.Type == "delta" {
			deltas += ev.Delta
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"list_articles"}, exec.calls)
	require.Contains(t, types, "tool_call")
	require.Contains(t, types, "tool_result")
	require.Contains(t, types, "delta")
	require.Contains(t, deltas, "Found 1 article")
	require.Equal(t, 2, client.calls)
}

func TestArticleToolDefs_SixTools(t *testing.T) {
	defs := llm.ArticleToolDefs()
	require.Len(t, defs, 6)
	names := map[string]bool{}
	for _, d := range defs {
		require.Equal(t, "function", d.Type)
		names[d.Function.Name] = true
	}
	require.True(t, names["list_articles"])
	require.True(t, names["create_article"])
}
