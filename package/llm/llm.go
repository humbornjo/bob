package llm

import (
	"context"
	"errors"
	"iter"
	"strings"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers"
)

const (
	PROVIDER_OPENAI Provider = "openai"
	PROVIDER_GEMINI Provider = "gemini"
)

var (
	ErrHumanInTheLoop = errors.New("#HITL")
)

func Completion(ctx context.Context, provider anyllm.Provider, params anyllm.CompletionParams,
) (*providers.ChatCompletion, error) {
	return provider.Completion(ctx, params)
}

func CompletionStream(ctx context.Context, provider anyllm.Provider, params anyllm.CompletionParams,
) iter.Seq2[anyllm.ChatCompletionChunk, error] {
	streamf := func(ctx context.Context, params anyllm.CompletionParams,
	) iter.Seq2[providers.ChatCompletionChunk, error] {
		chanChunks, chanErrs := provider.CompletionStream(ctx, params)
		return func(yield func(providers.ChatCompletionChunk, error) bool) {
			for chunk := range chanChunks {
				if !yield(chunk, nil) {
					return
				}
			}
			if err := <-chanErrs; err != nil {
				yield(providers.ChatCompletionChunk{}, err)
				return
			}
		}
	}

	return streamf(ctx, params)
}

func Agent(ctx context.Context, provider anyllm.Provider, params *anyllm.CompletionParams,
) iter.Seq2[*providers.ChatCompletion, error] {
	step := 0
	return func(yield func(*providers.ChatCompletion, error) bool) {
		for {
			completion, err := provider.Completion(ctx, *params)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(completion, err) {
				return
			}
			step++
		}
	}
}

func AgentStream(ctx context.Context, provider anyllm.Provider, params *anyllm.CompletionParams,
) iter.Seq2[int, iter.Seq2[providers.ChatCompletionChunk, error]] {
	step := 0
	return func(yield func(int, iter.Seq2[providers.ChatCompletionChunk, error]) bool) {
	loop:
		if !yield(step, CompletionStream(ctx, provider, *params)) {
			return
		}
		step++
		goto loop
	}
}

type Summarizer struct {
	usage anyllm.Usage

	content       strings.Builder
	reasonContent strings.Builder

	toolCalls        map[string]anyllm.ToolCall
	toolArgsBuilders map[string]*strings.Builder

	messages []anyllm.Message
}

func (s *Summarizer) Collect(chunk anyllm.ChatCompletionChunk) {
	if s.toolCalls == nil {
		s.toolCalls = make(map[string]anyllm.ToolCall)
		s.toolArgsBuilders = make(map[string]*strings.Builder)
	}

	usage := chunk.Usage
	if usage != nil {
		s.usage.TotalTokens += usage.TotalTokens
		s.usage.PromptTokens += usage.PromptTokens
		s.usage.CompletionTokens += usage.CompletionTokens
		s.usage.ReasoningTokens += usage.ReasoningTokens
	}

	delta := chunk.Choices[0].Delta

	s.content.WriteString(delta.Content)
	if delta.Reasoning != nil {
		s.reasonContent.WriteString(delta.Reasoning.Content)
	}

	for _, tc := range delta.ToolCalls {
		id := tc.ID
		if _, ok := s.toolCalls[id]; !ok {
			s.toolCalls[id] = tc
			s.toolArgsBuilders[id] = &strings.Builder{}
		}
		s.toolArgsBuilders[id].WriteString(tc.Function.Arguments)
	}
}

func (s *Summarizer) DrainStep() (message anyllm.Message, toolCalls iter.Seq2[string, anyllm.ToolCall]) {
	defer s.content.Reset()
	defer s.reasonContent.Reset()

	content := s.content.String()
	reasonContent := s.reasonContent.String()

	message = anyllm.Message{
		Role:    anyllm.RoleAssistant,
		Content: content,
		Reasoning: &anyllm.Reasoning{
			Content: reasonContent,
		},
	}

	s.messages = append(s.messages, message)
	for id, tc := range s.toolCalls {
		tc.Function.Arguments = s.toolArgsBuilders[id].String()
		message.ToolCalls = append(message.ToolCalls, tc)
	}

	s.toolCalls = make(map[string]anyllm.ToolCall)
	s.toolArgsBuilders = make(map[string]*strings.Builder)

	return message, func(yield func(string, anyllm.ToolCall) bool) {
		for _, tc := range message.ToolCalls {
			if !yield(tc.ID, tc) {
				return
			}
		}
	}
}

func (s *Summarizer) Messages() []anyllm.Message {
	return s.messages
}

func (s *Summarizer) AppendMessages(msgs ...anyllm.Message) {
	s.messages = append(s.messages, msgs...)
}
