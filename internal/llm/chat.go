package llm

import (
	"errors"
	"strings"
)

// MaxChatHistory is the maximum prior messages kept (excluding system + latest user).
const MaxChatHistory = 40

// ErrInvalidChat is returned for malformed playground chat payloads.
var ErrInvalidChat = errors.New("llm: invalid chat messages")

// BuildChatMessages assembles OpenAI-compatible messages for the streaming playground.
// history may contain prior user/assistant turns only (system is taken from systemPrompt).
func BuildChatMessages(systemPrompt string, history []Message, userMessage string) ([]Message, error) {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return nil, ErrEmptyBody
	}
	if len(userMessage) > MaxBodyBytes {
		userMessage = ClipBody(userMessage)
	}

	out := make([]Message, 0, len(history)+2)
	if sys := strings.TrimSpace(systemPrompt); sys != "" {
		if len(sys) > MaxBodyBytes {
			sys = ClipBody(sys)
		}
		out = append(out, Message{Role: "system", Content: sys})
	}

	// Keep only the latest MaxChatHistory history messages.
	if len(history) > MaxChatHistory {
		history = history[len(history)-MaxChatHistory:]
	}
	for _, m := range history {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		switch role {
		case "user", "assistant":
			if len(content) > MaxBodyBytes {
				content = ClipBody(content)
			}
			out = append(out, Message{Role: role, Content: content})
		case "system":
			// Ignore embedded system in history; top-level systemPrompt wins.
			continue
		default:
			return nil, ErrInvalidChat
		}
	}
	out = append(out, Message{Role: "user", Content: userMessage})
	return out, nil
}
