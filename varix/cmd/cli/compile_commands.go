package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	c "github.com/kumaloha/VariX/varix/compile"
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type compileClient interface {
	Compile(ctx context.Context, bundle c.Bundle) (c.Record, error)
	Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error)
	VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error)
}

var buildCompileClient = func(projectRoot string) compileClient {
	return c.NewClientFromConfig(projectRoot, nil)
}

var buildCompileClientNoVerify = func(projectRoot string) compileClient {
	return c.NewClientFromConfigNoVerify(projectRoot, nil)
}

var buildCompileClientV2 = func(projectRoot string) compileClient {
	return cv2.NewClientFromConfig(projectRoot, nil)
}

var openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
	return contentstore.NewSQLiteStore(path)
}

const compileCommandUsage = "usage: varix compile <run|batch-run|validate-run|show|summary|compare|card|gold-score> ..."

func selectCompileClient(projectRoot, pipeline string, noVerify bool) (compileClient, error) {
	switch strings.TrimSpace(pipeline) {
	case "", "legacy":
		if noVerify {
			return buildCompileClientNoVerify(projectRoot), nil
		}
		return buildCompileClient(projectRoot), nil
	case "v2":
		if noVerify {
			return nil, fmt.Errorf("--no-verify is not supported with --pipeline v2")
		}
		return buildCompileClientV2(projectRoot), nil
	default:
		return nil, fmt.Errorf("unsupported compile pipeline")
	}
}

func parseCompileLLMCacheMode(value string) (contentstore.LLMCacheMode, error) {
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
	case "gold-score":
		return runCompileGoldScore(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, compileCommandUsage)
		return 2
	}
}
