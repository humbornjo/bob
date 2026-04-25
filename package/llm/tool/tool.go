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
	Name() string
	Tool() anyllm.Tool
	Function() anyllm.Function
	Execute(ctx context.Context, args string, opts ...Option) (string, error)
	ExecuteStream(ctx context.Context, args string, opts ...Option) iter.Seq2[string, error]
}

func Wizard(functionTools ...Toolx,
) ([]anyllm.Tool, func(context.Context, anyllm.FunctionCall, ...Option) (string, error)) {
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

type ToolCallCollector struct {
	it iter.Seq2[string, anyllm.ToolCall]

	toolcalls map[string]anyllm.ToolCall
	builders  map[string]*strings.Builder
}

func NewToolCallCollector() ToolCallCollector {
	builders := make(map[string]*strings.Builder)
	toolcalls := make(map[string]anyllm.ToolCall)
	return ToolCallCollector{
		it: func(yield func(string, anyllm.ToolCall) bool) {
			for id, tc := range toolcalls {
				tc.Function.Arguments = builders[id].String()
				if !yield(id, tc) {
					return
				}
			}
		},
		builders: builders, toolcalls: toolcalls,
	}
}

func (tcc ToolCallCollector) Collect(tc anyllm.ToolCall) {
	id := tc.ID
	if _, ok := tcc.toolcalls[id]; !ok {
		tcc.toolcalls[id] = tc
		tcc.builders[id] = &strings.Builder{}
	}
	tcc.builders[id].WriteString(tc.Function.Arguments)
}

func (tcc ToolCallCollector) Iterate() iter.Seq2[string, anyllm.ToolCall] {
	return tcc.it
}
