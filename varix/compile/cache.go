package compile

import (
	"context"

	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/forge/llm"
)

type stageRuntime interface {
	CallStage(ctx context.Context, stageName string, req llm.ProviderRequest) (llm.Response, error)
}

func newCachedRuntime(next runtimeChat, store varixllm.CacheStore, mode varixllm.CacheMode) runtimeChat {
	return varixllm.NewCachedRuntime(next, store, mode, "compile")
}

func callStageRuntime(ctx context.Context, rt runtimeChat, stageName string, req llm.ProviderRequest) (llm.Response, error) {
	return varixllm.CallStage(ctx, rt, stageName, req)
}
