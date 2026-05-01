package compilev2

import (
	"context"
	"github.com/kumaloha/forge/llm"
)

type fakeRuntime struct {
	responses []llm.Response
	requests  []llm.ProviderRequest
	calls     int
}

func (f *fakeRuntime) Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error) {
	f.requests = append(f.requests, req)
	if f.calls >= len(f.responses) {
		return llm.Response{}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}
