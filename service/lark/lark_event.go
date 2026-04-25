package larksvc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/chyroc/lark"
	"github.com/humbornjo/bob/package/llm"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
	larksock "github.com/humbornjo/bob/service/lark/sock"
	larktool "github.com/humbornjo/bob/service/lark/tool"
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

	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()

		mentions := make([]*lark.Mention, len(event.Event.Message.Mentions))
		for i, mention := range event.Event.Message.Mentions {
			mentions[i] = &lark.Mention{
				Key: *mention.Key, ID: *mention.Id.UserId, IDType: lark.IDTypeUserID, Name: *mention.Name,
			}
		}

		userMessage, err := s.BuildMessageUser(
			ctx, content, event.Event.Message.ChatId, event.Event.Message.ThreadId, mentions...,
		)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build user message", "err", err)
			return
		}
		messages := []anyllm.Message{s.BuildMessageSystem(), userMessage}

		params := anyllm.CompletionParams{
			Model:      s.model,
			Messages:   messages,
			ToolChoice: "required",
			Tools:      []anyllm.Tool{larktool.NewCreateMessageSend().Tool(), larktool.NewCreateMessageReply().Tool()},
		}

		var socket larksock.Socket
		completion, err := llm.Completion(ctx, params)
		if err != nil {
			slog.ErrorContext(ctx, "failed to call chat completion", "err", err)
			return
		}
		switch completion.Choices[0].Message.ToolCalls[0].Function.Name {
		case larktool.NewCreateMessageSend().Name():
			socket = larksock.NewSendSocket(s.larkcli, *event.Event.Message.ChatId)
		case larktool.NewCreateMessageReply().Name():
			socket = larksock.NewReplySocket(s.larkcli, *event.Event.Message.MessageId)
		}
		defer socket.Close(ctx, nil) // nolint: errcheck

		toolxs, err := s.BuildToolsetPredefined(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to build predefined toolset", "err", err)
			return
		}
		tools, toolsHandler := llmtool.Wizard(toolxs...)

		params.Stream = true
		params.Tools = tools
		params.ToolChoice = "auto"
		params.ParallelToolCalls = new(true)
		params.ReasoningEffort = anyllm.ReasoningEffortMedium

		sb := &strings.Builder{}
		for step, stream := range llm.AgentStream(ctx, &params) {
			slog.DebugContext(ctx, "agent loop", "step", step)
			// Prevent infinite loop
			if step > 100 {
				params.Tools, params.ToolChoice = nil, ""
			}

			tcc := llmtool.NewToolCallCollector()
			for chunk, err := range stream {
			prologue:
				if err != nil {
					slog.ErrorContext(ctx, "agent loop exit on error", "err", err)
					if err := socket.Close(ctx, err); err != nil {
						slog.ErrorContext(ctx, "failed to send error", "err", err)
					}
					return
				}
				if content := chunk.Choices[0].Delta.Content; content != "" {
					sb.WriteString(content)
				}
				for _, toolcall := range chunk.Choices[0].Delta.ToolCalls {
					tcc.Collect(toolcall)
				}

				switch chunk.Choices[0].FinishReason {
				case anyllm.FinishReasonLength:
					err = errors.New("message too long")
					goto prologue
				case anyllm.FinishReasonContentFilter:
					err = errors.New("content filtered")
					goto prologue
				case anyllm.FinishReasonToolCalls:
				case anyllm.FinishReasonStop:
					if err := socket.Close(ctx, socket.Send(ctx, &lark.MessageContentPostMD{Text: sb.String()})); err != nil {
						slog.ErrorContext(ctx, "failed to close with error", "err", err)
					}
					return
				}
			}

			assistantMessage := anyllm.Message{Role: anyllm.RoleAssistant}
			if stepContent := sb.String(); stepContent != "" {
				sb.Reset()
				assistantMessage.Content = stepContent
				if err := socket.Close(ctx, socket.Send(ctx, &lark.MessageContentPostMD{Text: stepContent})); err != nil {
					slog.ErrorContext(ctx, "failed to close with error", "err", err)
					return
				}
			}

			toolcallMessages := []anyllm.Message{}
			for id, tc := range tcc.Iterate() {
				slog.DebugContext(ctx, "toolcall start", "id", id, "function", tc.Function.Name, "args", tc.Function.Arguments)
				assistantMessage.ToolCalls = append(assistantMessage.ToolCalls, tc)
				result, err := toolsHandler(ctx, tc.Function, llmtool.WithMetadata(metadata))
				if err != nil {
					slog.ErrorContext(
						ctx, "failed to execute function tool",
						"error", err, "toolcall_id", id, "function", tc.Function.Name, "arguments", tc.Function.Arguments,
					)
					continue
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
