package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	verification "github.com/kumaloha/VariX/varix/verify"
)

type compileClient interface {
	Compile(ctx context.Context, bundle model.Bundle) (model.Record, error)
}

type compilePreviewRenderer interface {
	RenderPreview(ctx context.Context, bundle model.Bundle, result c.FlowPreviewResult) (c.FlowPreviewResult, error)
}

type verifyClient interface {
	VerifyDetailed(ctx context.Context, bundle model.Bundle, output model.Output) (model.Verification, error)
}

var buildCompileClientCurrent = func(projectRoot string) compileClient {
	return c.NewClientFromConfig(projectRoot, nil)
}

var buildCompilePreviewRenderer = func(projectRoot string) compilePreviewRenderer {
	return c.NewClientFromConfig(projectRoot, nil)
}

var buildVerifyClient = func(projectRoot string) verifyClient {
	return verification.NewClientFromConfig(projectRoot, nil)
}

var openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
	return contentstore.NewSQLiteStore(path)
}

const compileCommandUsage = "usage: varix compile <run|rerender|sweep|batch-run|validate-run|show|summary|compare|card> ..."

func parseLLMCacheMode(value string) (contentstore.LLMCacheMode, error) {
	switch strings.TrimSpace(value) {
	case "", string(contentstore.LLMCacheReadThrough):
		return contentstore.LLMCacheReadThrough, nil
	case string(contentstore.LLMCacheRefresh):
		return contentstore.LLMCacheRefresh, nil
	case string(contentstore.LLMCacheOff):
		return contentstore.LLMCacheOff, nil
	default:
		return "", fmt.Errorf("unsupported --llm-cache %q; supported: read-through, refresh, off", value)
	}
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func runCompileCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, compileCommandUsage)
		return 2
	}

	switch args[0] {
	case "run":
		return runCompileRun(args[1:], projectRoot, stdout, stderr)
	case "rerender":
		return runCompileRerender(args[1:], projectRoot, stdout, stderr)
	case "sweep":
		return runCompileSweep(args[1:], projectRoot, stdout, stderr)
	case "batch-run":
		return runCompileBatchRun(args[1:], projectRoot, stdout, stderr)
	case "validate-run":
		return runCompileValidateRun(args[1:], projectRoot, stdout, stderr)
	case "show":
		return runCompileShow(args[1:], projectRoot, stdout, stderr)
	case "summary":
		return runCompileSummary(args[1:], projectRoot, stdout, stderr)
	case "compare":
		return runCompileCompare(args[1:], projectRoot, stdout, stderr)
	case "card":
		return runCompileCard(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, compileCommandUsage)
		return 2
	}
}
