package larksvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	anyllm "github.com/mozilla-ai/any-llm-go"

	"github.com/humbornjo/bob/package/llm"
	llmtool "github.com/humbornjo/bob/package/llm/tool"
)

func (s *Service) HandleP1MessageReadV1(ctx context.Context, event *larkim.P1MessageReadV1) error {
	return nil
}

func (s *Service) HandleP1MessageReceiveV1(ctx context.Context, event *larkim.P1MessageReceiveV1) error {
	return nil
}

func (s *Service) HandleP2MessageReadV1(ctx context.Context, event *larkim.P2MessageReadV1) error {
	return nil
}

func (s *Service) HandleP2MessageReceiveV1(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	eMessage, eSender := event.Event.Message, event.Event.Sender
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		complex, metadata, err := s.BuildMessages(ctx, eMessage, eSender)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build messages", "err", err)
			return
		}

		socket, params, err := s.BuildCompletion(ctx, complex.Messages, eMessage)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build completion params", "err", err)
			return
		}

		tools, handler, err := s.BuildToolset(ctx)
		if err == nil {
			params.Tools = tools
		} else {
			slog.ErrorContext(ctx, "failed to build predefined toolset", "err", err)
			return
		}

		var summarizer llm.Summarizer
		summarizer.AppendMessages(complex.UserMessage) // Append user messages only, exclude history and system

		// nolint: errcheck // Save the llm result to persistence
		defer s.FinalizeMessages(ctx, *eMessage.ChatId, *eMessage.MessageId, &summarizer)
	agent:
		for step, stream := range llm.AgentStream(ctx, s.provider, &params) {
			slog.DebugContext(ctx, "agent loop", "step", step)
			// Prevent infinite loop
			if step > 100 {
				params.Tools, params.ToolChoice = nil, ""
			}

			for chunk, err := range stream {
			stream_prologue:
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						slog.ErrorContext(ctx, "agent loop deadline exceeded, retrying...", "step", step)
						continue agent
					}
					slog.ErrorContext(ctx, "agent loop exit on error", "err", err)
					_, _ = fmt.Fprint(socket, err.Error())
					return
				}
				summarizer.Collect(chunk)

				switch chunk.Choices[0].FinishReason {
				case anyllm.FinishReasonLength:
					err = errors.New("message too long")
					goto stream_prologue
				case anyllm.FinishReasonContentFilter:
					err = errors.New("content filtered")
					goto stream_prologue
				case anyllm.FinishReasonToolCalls:
				case anyllm.FinishReasonStop:
					message, _ := summarizer.DrainStep()
					_, _ = fmt.Fprint(socket, message.Content.(string))
					return
				}
			}

			assistantMessage, toolCalls := summarizer.DrainStep()
			if content, ok := assistantMessage.Content.(string); ok && content != "" {
				assistantMessage.Content = content
				_, _ = fmt.Fprint(socket, content)
			}

			toolcallMessages := []anyllm.Message{}
			for id, tc := range toolCalls {
				slog.DebugContext(ctx, "toolcall start", "id", id, "function", tc.Function.Name, "args", tc.Function.Arguments)
				assistantMessage.ToolCalls = append(assistantMessage.ToolCalls, tc)
				result, err := handler(ctx, tc.Function, llmtool.WithMetadata(metadata))
				if err != nil {
					slog.ErrorContext(
						ctx, "failed to execute function tool",
						"error", err, "toolcall_id", id, "function", tc.Function.Name, "arguments", tc.Function.Arguments,
					)
					result = err.Error()
				}
				slog.DebugContext(ctx, "toolcall end", "id", id, "function", tc.Function.Name, "result", result)
				toolcallMessages = append(toolcallMessages, anyllm.Message{Role: anyllm.RoleTool, ToolCallID: id, Content: result})
			}
			slog.DebugContext(ctx, "toolcall messages", "count", len(toolcallMessages))
			summarizer.AppendMessages(toolcallMessages...)
			params.Messages = append(params.Messages, assistantMessage)
			params.Messages = append(params.Messages, toolcallMessages...)

			params.Messages, err = s.TidyMessages(ctx, params.Messages)
			if err != nil {
				slog.ErrorContext(ctx, "failed to tidy messages", "err", err)
			}
		}
	}()

	return nil
}
