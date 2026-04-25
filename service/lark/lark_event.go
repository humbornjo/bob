package larksvc

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/chyroc/lark"
	"github.com/humbornjo/bob/package/llm"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	anyllm "github.com/mozilla-ai/any-llm-go"

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
	content, err := lark.UnwrapMessageContent(lark.MsgType(*event.Event.Message.MessageType), *event.Event.Message.Content)
	if err != nil {
		slog.ErrorContext(ctx, "failed to unwrap message content", "err", err, "request_id", event.RequestId())
		return err
	}

	metadata := map[string]any{}
	if event.Event.Message.ChatId != nil {
		metadata["chat_id"] = *event.Event.Message.ChatId
	}
	if event.Event.Message.ThreadId != nil {
		metadata["thread_id"] = *event.Event.Message.ThreadId
	}

	mentions := make([]*lark.Mention, 0, len(event.Event.Message.Mentions))
	for _, mention := range event.Event.Message.Mentions {
		var id string
		var idtype lark.IDType
		switch {
		case *mention.Id.UserId != "":
			id, idtype = *mention.Id.UserId, lark.IDTypeUserID
		case *mention.Id.OpenId != "":
			id, idtype = *mention.Id.OpenId, lark.IDTypeOpenID
		case *mention.Id.UnionId != "":
			id, idtype = *mention.Id.UnionId, lark.IDTypeUnionID
		}
		mentions = append(mentions, &lark.Mention{Key: *mention.Key, ID: id, IDType: idtype, Name: *mention.Name})
	}
	if *event.Event.Message.ChatType != string(lark.ChatModeP2P) &&
		!slices.ContainsFunc(mentions, func(mention *lark.Mention) bool { return mention.ID == s.openid }) {
		return nil
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		userMessage, err := s.BuildMessageUser(
			ctx, content, event.Event.Message.ChatId, event.Event.Message.ThreadId, mentions...,
		)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build user message", "err", err)
			return
		}
		messages := []anyllm.Message{s.BuildMessageSystem(), userMessage}

		socket, params, err := s.BuildCompletionParams(ctx, messages, event)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build completion params", "err", err)
			return
		}

		var functionHandler llmtool.FunctionHandler
		preToolxs, err := s.BuildToolsetPredefined(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build predefined toolset", "err", err)
			return
		}
		preTools, preFuncHandler := llmtool.Wizard(preToolxs...)
		params.Tools, functionHandler = preTools, preFuncHandler

		summ := llmtool.Summarizer{}
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
					if err := socket.Close(ctx, err); err != nil {
						slog.ErrorContext(ctx, "failed to send error", "err", err)
					}
					return
				}
				summ.Collect(chunk)

				switch chunk.Choices[0].FinishReason {
				case anyllm.FinishReasonLength:
					err = errors.New("message too long")
					goto stream_prologue
				case anyllm.FinishReasonContentFilter:
					err = errors.New("content filtered")
					goto stream_prologue
				case anyllm.FinishReasonToolCalls:
				case anyllm.FinishReasonStop:
					if err := socket.Close(ctx, socket.Send(ctx, &lark.MessageContentPostMD{
						Text: summ.DrainContent(),
					})); err != nil {
						slog.ErrorContext(ctx, "failed to close with error", "err", err)
					}
					return
				}
			}

			assistantMessage := anyllm.Message{Role: anyllm.RoleAssistant}
			if content := summ.DrainReasonContent(); content != "" {
				assistantMessage.Reasoning = &anyllm.Reasoning{Content: content}
			}
			if content := summ.DrainContent(); content != "" {
				assistantMessage.Content = content
				if err := socket.Close(ctx, socket.Send(ctx, &lark.MessageContentPostMD{Text: content})); err != nil {
					slog.ErrorContext(ctx, "failed to close with error", "err", err)
					return
				}
			}

			toolcallMessages := []anyllm.Message{}
			for id, tc := range summ.DrainToolCalls() {
				slog.DebugContext(ctx, "toolcall start", "id", id, "function", tc.Function.Name, "args", tc.Function.Arguments)
				assistantMessage.ToolCalls = append(assistantMessage.ToolCalls, tc)
				result, err := functionHandler(ctx, tc.Function, llmtool.WithMetadata(metadata))
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
			params.Messages = append(params.Messages, assistantMessage)
			params.Messages = append(params.Messages, toolcallMessages...)
		}
	}()

	return nil
}
