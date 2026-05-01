package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runMemoryEventGraphs(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory event-graphs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute event graph projection before reading")
	scope := fs.String("scope", "", "optional filter: driver or target")
	subject := fs.String("subject", "", "optional filter on anchor subject")
	card := fs.Bool("card", false, "render a readable event graph card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory event-graphs --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedScope := strings.TrimSpace(*scope)
	trimmedSubject := strings.TrimSpace(*subject)
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *runNow {
		if _, err := store.RunEventGraphProjection(context.Background(), trimmedUserID, currentUTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	var items []contentstore.EventGraphRecord
	if trimmedSubject != "" {
		items, err = store.ListEventGraphsBySubject(context.Background(), trimmedUserID, trimmedSubject)
	} else {
		items, err = store.ListEventGraphs(context.Background(), trimmedUserID)
	}
	if err == nil && trimmedScope != "" {
		filtered := make([]contentstore.EventGraphRecord, 0, len(items))
		for _, item := range items {
			if item.Scope == trimmedScope {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No event graphs matched")
			return 0
		}
		fmt.Fprint(stdout, formatEventGraphCards(items))
		return 0
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
func runMemoryParadigms(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory paradigms", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute paradigm projection before reading")
	subject := fs.String("subject", "", "optional filter on driver subject")
	card := fs.Bool("card", false, "render a readable paradigm card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory paradigms --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedSubject := strings.TrimSpace(*subject)
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *runNow {
		if _, err := store.RunParadigmProjection(context.Background(), trimmedUserID, currentUTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListParadigms(context.Background(), trimmedUserID)
	if err == nil && trimmedSubject != "" {
		items, err = store.ListParadigmsBySubject(context.Background(), trimmedUserID, trimmedSubject)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No paradigms matched")
			return 0
		}
		fmt.Fprint(stdout, formatParadigmCards(items))
		return 0
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
func runMemoryContentGraphs(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory content-graphs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "rebuild one snapshot from current compiled output before reading")
	card := fs.Bool("card", false, "render a readable content graph card view")
	subject := fs.String("subject", "", "optional filter on subject")
	platform := fs.String("platform", "", "source platform (required with --run)")
	externalID := fs.String("id", "", "source external id (required with --run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory content-graphs --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *runNow {
		if !hasContentTarget(*platform, *externalID) {
			fmt.Fprintln(stderr, "usage: varix memory content-graphs --run --user <user_id> --platform <platform> --id <external_id>")
			return 2
		}
		if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), currentUTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListMemoryContentGraphs(context.Background(), strings.TrimSpace(*userID))
	if err == nil && hasContentTarget(*platform, *externalID) {
		filtered := make([]graphmodel.ContentSubgraph, 0, len(items))
		for _, item := range items {
			if item.SourcePlatform == strings.TrimSpace(*platform) && item.SourceExternalID == strings.TrimSpace(*externalID) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err == nil && strings.TrimSpace(*subject) != "" {
		resolvedSubject, resolveErr := store.FindCanonicalEntityByAlias(context.Background(), strings.TrimSpace(*subject))
		if resolveErr == nil && strings.TrimSpace(resolvedSubject.CanonicalName) != "" {
			*subject = strings.TrimSpace(resolvedSubject.CanonicalName)
		} else if resolveErr != nil && !errors.Is(resolveErr, sql.ErrNoRows) {
			fmt.Fprintln(stderr, resolveErr)
			return 1
		}
		filtered := make([]graphmodel.ContentSubgraph, 0, len(items))
		for _, item := range items {
			matched := false
			for _, node := range item.Nodes {
				if node.SubjectText == strings.TrimSpace(*subject) || node.SubjectCanonical == strings.TrimSpace(*subject) {
					matched = true
					break
				}
			}
			if matched {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No content graphs matched")
			return 0
		}
		fmt.Fprint(stdout, formatContentGraphCards(items))
		return 0
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
