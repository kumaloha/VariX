package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"github.com/kumaloha/VariX/varix/model"
	"io"
	"strings"
)

func runCompileCard(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile card", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	compact := fs.Bool("compact", false, "render a compact product-facing card")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile card --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	app, store, err := openRuntimeStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	*platform, *externalID, err = resolveContentTarget(context.Background(), app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	var subgraph *model.ContentSubgraph
	if graph, graphErr := store.GetContentSubgraph(context.Background(), *platform, *externalID); graphErr == nil {
		subgraph = &graph
	} else if !errors.Is(graphErr, sql.ErrNoRows) {
		fmt.Fprintln(stderr, graphErr)
		return 1
	}
	projection := buildCompileCardProjection(record, subgraph)

	if *compact {
		fmt.Fprint(stdout, formatCompactCompileCard(projection))
		return 0
	}
	fmt.Fprint(stdout, formatCompileCard(projection))
	return 0
}
