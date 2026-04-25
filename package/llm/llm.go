package llm

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"os"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
)

var (
	ErrHumanInTheLoop = errors.New("#HITL")
)

var _PROVIDER anyllm.Provider

func init() {
	// Initialize the provider
	apiKey, baseUrl := os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_BASE_URL")
	provider, err := openai.New(config.WithAPIKey(apiKey), config.WithBaseURL(baseUrl))
	if err != nil {
		panic(err)
	}

	_PROVIDER = provider
}

func Completion(ctx context.Context, params anyllm.CompletionParams) (*providers.ChatCompletion, error) {
	return _PROVIDER.Completion(ctx, params)
}

func CompletionStream(ctx context.Context, params anyllm.CompletionParams) iter.Seq2[anyllm.ChatCompletionChunk, error] {
	streamf := func(ctx context.Context, params anyllm.CompletionParams) iter.Seq2[providers.ChatCompletionChunk, error] {
		chanChunks, chanErrs := _PROVIDER.CompletionStream(ctx, params)
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

func Agent(ctx context.Context, params *anyllm.CompletionParams) iter.Seq2[*providers.ChatCompletion, error] {
	step := 0
	return func(yield func(*providers.ChatCompletion, error) bool) {
		for {
			slog.DebugContext(ctx, "#llm agent loop", "model", params.Model, "step", step)
			completion, err := _PROVIDER.Completion(ctx, *params)
			if err != nil {
				slog.DebugContext(ctx, "#llm agent loop exit on error", "model", params.Model, "step", step, "err", err)
				yield(nil, err)
				return
			}
			slog.DebugContext(ctx, "#llm agent loop yield", "model", params.Model, "step", step)
			if !yield(completion, err) {
				slog.Debug("#llm agent loop exit on interrupt", "model", params.Model, "step", step)
				return
			}
			slog.DebugContext(ctx, "#llm agent loop done yield", "model", params.Model, "step", step)
			step++
		}
	}
}

func AgentStream(ctx context.Context, params *anyllm.CompletionParams,
) iter.Seq2[int, iter.Seq2[providers.ChatCompletionChunk, error]] {
	step := 0
	return func(yield func(int, iter.Seq2[providers.ChatCompletionChunk, error]) bool) {
	loop:
		if !yield(step, CompletionStream(ctx, *params)) {
			return
		}
		step++
		goto loop
	}
}
