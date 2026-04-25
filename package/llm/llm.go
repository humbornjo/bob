package llm

import (
	"context"
	"errors"
	"iter"

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
