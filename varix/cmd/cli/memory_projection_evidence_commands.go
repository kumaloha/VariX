package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runMemoryEventEvidence(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory event-evidence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	eventGraphID := fs.String("event-graph-id", "", "optional filter on one event graph id")
	card := fs.Bool("card", false, "render a readable event-evidence view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory event-evidence --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	links, err := store.ListEventGraphEvidenceLinksByUser(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*eventGraphID) != "" {
		filtered := make([]contentstore.EventGraphEvidenceLink, 0, len(links))
		for _, item := range links {
			if item.EventGraphID == strings.TrimSpace(*eventGraphID) {
				filtered = append(filtered, item)
			}
		}
		links = filtered
	}
	if len(links) == 0 {
		fmt.Fprintln(stdout, "No event evidence matched")
		return 0
	}
	if *card {
		fmt.Fprint(stdout, formatEventEvidenceCards(links))
		return 0
	}
	payload, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
func runMemoryParadigmEvidence(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory paradigm-evidence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	paradigmID := fs.String("paradigm-id", "", "optional filter on one paradigm id")
	card := fs.Bool("card", false, "render a readable paradigm-evidence view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory paradigm-evidence --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	links, err := store.ListParadigmEvidenceLinksByUser(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*paradigmID) != "" {
		filtered := make([]contentstore.ParadigmEvidenceLink, 0, len(links))
		for _, item := range links {
			if item.ParadigmID == strings.TrimSpace(*paradigmID) {
				filtered = append(filtered, item)
			}
		}
		links = filtered
	}
	if len(links) == 0 {
		fmt.Fprintln(stdout, "No paradigm evidence matched")
		return 0
	}
	if *card {
		fmt.Fprint(stdout, formatParadigmEvidenceCards(links))
		return 0
	}
	payload, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
