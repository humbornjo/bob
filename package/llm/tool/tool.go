package llmtool

import (
	"context"
	"fmt"
	"iter"

	anyllm "github.com/mozilla-ai/any-llm-go"
)

var (
	ErrToolNotExecutable = fmt.Errorf("tool not executable")
)

type Params interface {
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
