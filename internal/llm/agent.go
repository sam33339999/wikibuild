package llm

import (
	"context"
	"fmt"
)

// MaxToolRounds caps tool-call loops in the playground agent.
const MaxToolRounds = 8

// ToolExecutor runs a function tool by name with JSON arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, name, argumentsJSON string) (resultJSON string, err error)
}

// AgentEvent is emitted during a tool-using chat (for SSE).
type AgentEvent struct {
	Type    string // delta | tool_call | tool_result | error
	Delta   string `json:",omitempty"`
	Name    string `json:",omitempty"`
	ID      string `json:",omitempty"`
	Args    string `json:",omitempty"`
	Result  string `json:",omitempty"`
	Error   string `json:",omitempty"`
}

// ChatWithToolsClient is the subset needed for non-stream tool loops.
type ChatWithToolsClient interface {
	Enabled() bool
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (ChatResult, error)
}

// RunAgent loops chat→tools until a final text answer (or max rounds).
// Final assistant text is emitted as delta events (one chunk for simplicity, or split).
func RunAgent(ctx context.Context, client ChatWithToolsClient, exec ToolExecutor, messages []Message, tools []ToolDef, onEvent func(AgentEvent) error) error {
	if client == nil || !client.Enabled() {
		return ErrNotConfigured
	}
	if onEvent == nil {
		onEvent = func(AgentEvent) error { return nil }
	}
	msgs := append([]Message(nil), messages...)

	for round := 0; round < MaxToolRounds; round++ {
		res, err := client.Chat(ctx, msgs, tools)
		if err != nil {
			return err
		}
		if len(res.ToolCalls) == 0 {
			// Final answer — stream as deltas (chunk for UI feel).
			return emitContentDeltas(res.Content, onEvent)
		}

		// Append assistant message with tool_calls (OpenAI format via our Message extension).
		msgs = append(msgs, Message{
			Role:      "assistant",
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		})

		for _, tc := range res.ToolCalls {
			if err := onEvent(AgentEvent{Type: "tool_call", ID: tc.ID, Name: tc.Name, Args: tc.Arguments}); err != nil {
				return err
			}
			result, err := exec.Execute(ctx, tc.Name, tc.Arguments)
			if err != nil {
				result = fmt.Sprintf(`{"error":%q}`, err.Error())
			}
			if err := onEvent(AgentEvent{Type: "tool_result", ID: tc.ID, Name: tc.Name, Result: result}); err != nil {
				return err
			}
			msgs = append(msgs, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
		}
	}
	return fmt.Errorf("%w: max tool rounds exceeded", ErrBadResponse)
}

func emitContentDeltas(content string, onEvent func(AgentEvent) error) error {
	if content == "" {
		return nil
	}
	// Chunk ~40 runes for progressive UI without true token stream.
	runes := []rune(content)
	const chunk = 48
	for i := 0; i < len(runes); i += chunk {
		j := i + chunk
		if j > len(runes) {
			j = len(runes)
		}
		if err := onEvent(AgentEvent{Type: "delta", Delta: string(runes[i:j])}); err != nil {
			return err
		}
	}
	return nil
}

// MessageWithToolsJSON is used when marshaling tool-bearing messages to the API.
func appendToolMessages(apiMsgs []map[string]any, messages []Message) []map[string]any {
	for _, m := range messages {
		switch m.Role {
		case "tool":
			apiMsgs = append(apiMsgs, map[string]any{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"content":      m.Content,
			})
		case "assistant":
			msg := map[string]any{"role": "assistant", "content": m.Content}
			if len(m.ToolCalls) > 0 {
				tcs := make([]map[string]any, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					tcs = append(tcs, map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]string{
							"name":      tc.Name,
							"arguments": tc.Arguments,
						},
					})
				}
				msg["tool_calls"] = tcs
				if m.Content == "" {
					msg["content"] = nil
				}
			}
			apiMsgs = append(apiMsgs, msg)
		default:
			apiMsgs = append(apiMsgs, map[string]any{"role": m.Role, "content": m.Content})
		}
	}
	return apiMsgs
}

