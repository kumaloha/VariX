package main

import "strings"

var ingestCommands = []string{
	"fetch",
	"follow",
	"list-authors",
	"list-follows",
	"poll",
	"provenance-run",
}

var compileCommands = []string{
	"run",
	"sweep",
	"batch-run",
	"show",
	"summary",
	"compare",
	"card",
}

var verifyCommands = []string{
	"run",
	"show",
	"queue",
	"sweep",
}

var memoryCommands = []string{
	"accept",
	"accept-batch",
	"list",
	"show-source",
	"content-graphs",
	"subject-timeline",
	"subject-horizon",
	"subject-experience",
	"jobs",
	"posterior-run",
	"organize-run",
	"organized",
	"global-organize-run",
	"global-organized",
	"global-synthesis-run",
	"global-synthesis",
	"global-card",
	"global-synthesis-card",
	"global-compare",
	"event-graphs",
	"event-evidence",
	"paradigms",
	"paradigm-evidence",
	"project-all",
	"backfill",
	"cleanup-stale",
	"canonical-entities",
	"canonical-entity-upsert",
}

func isIngestCommand(name string) bool {
	for _, cmd := range ingestCommands {
		if name == cmd {
			return true
		}
	}
	return false
}

func usageText() string {
	return strings.Join([]string{
		"usage: varix <ingest|compile|verify|memory|serve>",
		"",
		"ingest: " + strings.Join(ingestCommands, "|"),
		"compile: " + strings.Join(compileCommands, "|"),
		"verify: " + strings.Join(verifyCommands, "|"),
		"memory: " + strings.Join(memoryCommands, "|"),
		"serve: --addr <host:port>",
	}, "\n")
}
