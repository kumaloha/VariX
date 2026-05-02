package main

import (
	"context"
	"encoding/json"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"os"
	"testing"
	"time"
)

type fakeItemSource struct {
	platform types.Platform
	kind     types.Kind
	items    []types.RawContent
	fetchFn  func(context.Context, types.ParsedURL) ([]types.RawContent, error)
}

func (f fakeItemSource) Platform() types.Platform {
	if f.platform != "" {
		return f.platform
	}
	return types.PlatformWeb
}

func (f fakeItemSource) Kind() types.Kind {
	if f.kind != "" {
		return f.kind
	}
	return types.KindNative
}

func (f fakeItemSource) Fetch(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, parsed)
	}
	return f.items, nil
}

type panicItemSource struct{}

func (panicItemSource) Platform() types.Platform { return types.PlatformWeb }

func (panicItemSource) Kind() types.Kind { return types.KindNative }

func (panicItemSource) Fetch(context.Context, types.ParsedURL) ([]types.RawContent, error) {
	panic("fetch should not be called")
}

type fakeCompileClient struct {
	record    c.Record
	err       error
	compileFn func(context.Context, c.Bundle) (c.Record, error)
	verifyFn  func(context.Context, c.Bundle, c.Output) (c.Verification, error)
}

func (f fakeCompileClient) Compile(ctx context.Context, bundle c.Bundle) (c.Record, error) {
	if f.compileFn != nil {
		return f.compileFn(ctx, bundle)
	}
	return f.record, f.err
}

func (f fakeCompileClient) Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	if f.verifyFn != nil {
		return f.verifyFn(ctx, bundle, output)
	}
	return c.Verification{}, f.err
}

func (f fakeCompileClient) VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	return f.Verify(ctx, bundle, output)
}

func testGraphNode(id string, kind c.NodeKind, text string) c.GraphNode {
	return c.GraphNode{
		ID:        id,
		Kind:      kind,
		Text:      text,
		ValidFrom: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		ValidTo:   time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

type compileClientStub struct {
	compile        func(context.Context, c.Bundle) (c.Record, error)
	verify         func(context.Context, c.Bundle, c.Output) (c.Verification, error)
	verifyDetailed func(context.Context, c.Bundle, c.Output) (c.Verification, error)
}

func (s compileClientStub) Compile(ctx context.Context, bundle c.Bundle) (c.Record, error) {
	if s.compile != nil {
		return s.compile(ctx, bundle)
	}
	return c.Record{}, nil
}

func (s compileClientStub) Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	if s.verify != nil {
		return s.verify(ctx, bundle, output)
	}
	return c.Verification{}, nil
}

func (s compileClientStub) VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error) {
	if s.verifyDetailed != nil {
		return s.verifyDetailed(ctx, bundle, output)
	}
	return c.Verification{}, nil
}

func writeTestJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
