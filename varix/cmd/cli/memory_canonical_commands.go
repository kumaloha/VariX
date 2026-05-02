package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

func runMemoryCanonicalEntities(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory canonical-entities", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entityID := fs.String("id", "", "optional canonical entity id filter")
	alias := fs.String("alias", "", "optional alias lookup filter")
	entityType := fs.String("type", "", "optional filter: driver | target | both")
	status := fs.String("status", "", "optional filter: active | merged | split | retired")
	card := fs.Bool("card", false, "render a readable canonical entity view")
	summary := fs.Bool("summary", false, "print aggregate counts instead of full entities")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	var items []memory.CanonicalEntity
	if strings.TrimSpace(*entityID) != "" {
		entity, err := store.GetCanonicalEntity(context.Background(), strings.TrimSpace(*entityID))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		items = []memory.CanonicalEntity{entity}
	} else if strings.TrimSpace(*alias) != "" {
		entity, err := store.FindCanonicalEntityByAlias(context.Background(), strings.TrimSpace(*alias))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		items = []memory.CanonicalEntity{entity}
	} else {
		items, err = store.ListCanonicalEntities(context.Background())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(*entityType) != "" {
		filtered := make([]memory.CanonicalEntity, 0, len(items))
		for _, item := range items {
			if string(item.EntityType) == strings.TrimSpace(*entityType) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if strings.TrimSpace(*status) != "" {
		filtered := make([]memory.CanonicalEntity, 0, len(items))
		for _, item := range items {
			if string(item.Status) == strings.TrimSpace(*status) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if *summary {
		byType := map[string]int{}
		byStatus := map[string]int{}
		totalAliases := 0
		for _, item := range items {
			byType[string(item.EntityType)]++
			byStatus[string(item.Status)]++
			totalAliases += len(item.Aliases)
		}
		payload, err := json.MarshalIndent(map[string]any{
			"total_entities": len(items),
			"total_aliases":  totalAliases,
			"by_type":        byType,
			"by_status":      byStatus,
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No canonical entities matched")
			return 0
		}
		var b strings.Builder
		for _, item := range items {
			fmt.Fprintf(&b, "Canonical Entity\n- entity_id: %s\n- canonical_name: %s\n- type: %s\n- status: %s\n", item.EntityID, item.CanonicalName, item.EntityType, item.Status)
			if len(item.Aliases) > 0 {
				fmt.Fprintf(&b, "- aliases: %s\n", strings.Join(item.Aliases, ", "))
			}
			b.WriteString("\n")
		}
		fmt.Fprint(stdout, b.String())
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

func runMemoryCanonicalEntityUpsert(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory canonical-entity-upsert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entityID := fs.String("id", "", "canonical entity id")
	entityType := fs.String("type", "", "driver | target | both")
	name := fs.String("name", "", "canonical display name")
	aliasesRaw := fs.String("aliases", "", "optional comma-separated aliases")
	status := fs.String("status", string(memory.CanonicalEntityActive), "active | merged | split | retired")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	entityIDValue := strings.TrimSpace(*entityID)
	entityTypeValue := strings.TrimSpace(*entityType)
	nameValue := strings.TrimSpace(*name)
	statusValue := strings.TrimSpace(*status)
	aliasesValue := strings.TrimSpace(*aliasesRaw)
	if entityIDValue == "" || entityTypeValue == "" || nameValue == "" {
		fmt.Fprintln(stderr, "usage: varix memory canonical-entity-upsert --id <entity_id> --type <driver|target|both> --name <canonical_name> [--aliases a,b]")
		return 2
	}
	var typ memory.CanonicalEntityType
	switch entityTypeValue {
	case string(memory.CanonicalEntityDriver):
		typ = memory.CanonicalEntityDriver
	case string(memory.CanonicalEntityTarget):
		typ = memory.CanonicalEntityTarget
	case string(memory.CanonicalEntityBoth):
		typ = memory.CanonicalEntityBoth
	default:
		fmt.Fprintln(stderr, "--type must be one of: driver, target, both")
		return 2
	}
	var entityStatus memory.CanonicalEntityStatus
	switch statusValue {
	case string(memory.CanonicalEntityActive):
		entityStatus = memory.CanonicalEntityActive
	case string(memory.CanonicalEntityMerged):
		entityStatus = memory.CanonicalEntityMerged
	case string(memory.CanonicalEntitySplit):
		entityStatus = memory.CanonicalEntitySplit
	case string(memory.CanonicalEntityRetired):
		entityStatus = memory.CanonicalEntityRetired
	default:
		fmt.Fprintln(stderr, "--status must be one of: active, merged, split, retired")
		return 2
	}
	aliases := make([]string, 0)
	for _, part := range strings.Split(aliasesValue, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			aliases = append(aliases, trimmed)
		}
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	entity := memory.CanonicalEntity{
		EntityID:      entityIDValue,
		EntityType:    typ,
		CanonicalName: nameValue,
		Aliases:       aliases,
		Status:        entityStatus,
	}
	if err := store.UpsertCanonicalEntity(context.Background(), entity); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(map[string]any{"ok": true, "entity_id": entity.EntityID}, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
