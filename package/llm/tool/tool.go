package llmtool

import (
	"context"
	"fmt"
	"iter"
	"strings"

	anyllm "github.com/mozilla-ai/any-llm-go"
)

var (
	ErrToolNotExecutable = fmt.Errorf("tool not executable")
)

type Params interface {
	fmt.Stringer
	Schema() map[string]any
}

type Option func(*config)

type config struct {
	Metadata map[string]any
}

func NewConfig(opts ...Option) *config {
	config := &config{Metadata: make(map[string]any)}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

func WithMetadata(metadata map[string]any) Option {
	return func(config *config) {
		config.Metadata = metadata
	}
}

type Toolx interface {
	Tool() anyllm.Tool
	Function() anyllm.Function
	Execute(ctx context.Context, args string, opts ...Option) (string, error)
	ExecuteStream(ctx context.Context, args string, opts ...Option) iter.Seq2[string, error]
}

type FunctionHandler = func(context.Context, anyllm.FunctionCall, ...Option) (string, error)

func Wizard(functionTools ...Toolx) ([]anyllm.Tool, FunctionHandler) {
	type handler = func(ctx context.Context, args string, opts ...Option) (string, error)

	n := len(functionTools)
	tools, dispatcher := make([]anyllm.Tool, 0, n), make(map[string]handler, n)

	for _, ftool := range functionTools {
		tools, dispatcher[ftool.Function().Name] = append(tools, ftool.Tool()), ftool.Execute
	}

	return tools, func(ctx context.Context, fc anyllm.FunctionCall, opts ...Option) (string, error) {
		functionName := fc.Name
		handler, ok := dispatcher[functionName]
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrToolNotExecutable, functionName)
		}
		return handler(ctx, fc.Arguments, opts...)
	}
}

type Summarizer struct {
	usage anyllm.Usage

	content       strings.Builder
	reasonContent strings.Builder

	toolCalls        map[string]anyllm.ToolCall
	toolArgsBuilders map[string]*strings.Builder
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

func (s *Summarizer) DrainContent() string {
	defer s.content.Reset()
	return s.content.String()
}

func (s *Summarizer) DrainReasonContent() string {
	defer s.reasonContent.Reset()
	return s.reasonContent.String()
}

func (s *Summarizer) DrainToolCalls() iter.Seq2[string, anyllm.ToolCall] {
	return func(yield func(string, anyllm.ToolCall) bool) {
		for id, tc := range s.toolCalls {
			tc.Function.Arguments = s.toolArgsBuilders[id].String()
			if !yield(id, tc) {
				return
			}
		}
	}
}
