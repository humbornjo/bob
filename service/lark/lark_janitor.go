package larksvc

import (
	"context"
	_ "embed"
	"html/template"
	"log/slog"

	"github.com/humbornjo/bob/package/llm"
	anyllm "github.com/mozilla-ai/any-llm-go"
)

var (
	//go:embed static/system_prompt_chat_profile.md
	_SYSTEM_PROMPT_CHAT_PROFILE      string
	_SYSTEM_PROMPT_CHAT_PROFILE_TMPL = template.Must(template.New("chat_profile").Parse(_SYSTEM_PROMPT_CHAT_PROFILE))
)

func (s *Service) TidyMessages(ctx context.Context, messages []anyllm.Message) ([]anyllm.Message, error) {
	return messages, nil
}

func (s *Service) FinalizeMessages(
	ctx context.Context,
	chatID string, messageID string, summarizer *llm.Summarizer) error {
	// Save messages to persistence
	if _, err := s.ps.SaveMessage(ctx, SERVICE_NAME, chatID, messageID, summarizer.Messages()); err != nil {
		slog.ErrorContext(ctx, "failed to save messages", "err", err)
		return err
	}

	// Get existing profile for this chat
	oldProfile, _ := s.ps.GetProfile(ctx, SERVICE_NAME, chatID)

	// Build summary prompt using BuildSystemMessage
	systemMessage := s.BuildSystemMessage(
		_SYSTEM_PROMPT_CHAT_PROFILE_TMPL,
		struct {
			OldProfile string
			IsUpdate   bool
		}{
			OldProfile: oldProfile,
			IsUpdate:   oldProfile != "",
		},
	)

	// Create completion params for summarization
	params := anyllm.CompletionParams{
		Model:    s.model,
		Messages: append([]anyllm.Message{systemMessage}, summarizer.Messages()...),
	}

	// Get summary from LLM
	completion, err := llm.Completion(ctx, s.provider, params)
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate profile summary", "err", err)
		return err
	}

	if len(completion.Choices) > 0 {
		if content, ok := completion.Choices[0].Message.Content.(string); ok && content != "" {
			if len(content) > 8192 {
				content = content[:8192]
			}
			if err := s.ps.UpdateProfile(ctx, SERVICE_NAME, chatID, content); err != nil {
				slog.ErrorContext(ctx, "failed to update profile", "err", err)
				return err
			}
			slog.DebugContext(ctx, "updated chat profile", "chat_id", chatID)
		}
	}

	return nil
}
