package main

import "strings"

var ingestCommands = []string{
	"fetch",
	"follow",
	"list-follows",
	"poll",
	"provenance-run",
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
	return "usage: cli <" + strings.Join(ingestCommands, "|") + ">"
}
