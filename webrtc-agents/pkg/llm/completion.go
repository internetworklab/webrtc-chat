package llm

import "context"

type CompletionGenerator interface {
	Generate(ctx context.Context, request OpenRouterCompletionRequest) OpenRouterResponse
}
