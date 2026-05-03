package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"sync"
)

type fakeRuntime struct {
	responses []llm.Response
	requests  []llm.ProviderRequest
	calls     int
	mu        sync.Mutex
}

func (f *fakeRuntime) Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.calls >= len(f.responses) {
		return llm.Response{}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}
