package main

import "strings"

var ingestCommands = []string{
	"fetch",
	"follow",
	"list-follows",
	"poll",
	"provenance-run",
}

var compileCommands = []string{
	"run",
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
	"jobs",
	"posterior-run",
	"organize-run",
	"organized",
	"global-organize-run",
	"global-organized",
	"global-v2-organize-run",
	"global-v2-organized",
	"global-card",
	"global-v2-card",
	"global-compare",
	"event-graphs",
	"event-evidence",
	"paradigms",
	"paradigm-evidence",
	"project-all",
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
		"usage: varix <ingest|compile|verify|memory>",
		"",
		"ingest: " + strings.Join(ingestCommands, "|"),
		"compile: " + strings.Join(compileCommands, "|"),
		"verify: " + strings.Join(verifyCommands, "|"),
		"memory: " + strings.Join(memoryCommands, "|"),
	}, "\n")
}
