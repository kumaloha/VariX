package main

import (
	"fmt"
	"io"
)

const memoryCommandUsage = "usage: varix memory <accept|accept-batch|list|show-source|content-graphs|subject-timeline|subject-horizon|subject-experience|jobs|posterior-run|organize-run|organized|global-organize-run|global-organized|global-synthesis-run|global-synthesis|global-card|global-synthesis-card|global-compare|event-graphs|event-evidence|paradigms|paradigm-evidence|project-all|projection-sweep|backfill|cleanup-stale|canonical-entities|canonical-entity-upsert> ..."

func runMemoryCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, memoryCommandUsage)
		return 2
	}
	switch args[0] {
	case "accept":
		return runMemoryAccept(args[1:], projectRoot, stdout, stderr)
	case "accept-batch":
		return runMemoryAcceptBatch(args[1:], projectRoot, stdout, stderr)
	case "list":
		return runMemoryList(args[1:], projectRoot, stdout, stderr)
	case "show-source":
		return runMemoryShowSource(args[1:], projectRoot, stdout, stderr)
	case "content-graphs":
		return runMemoryContentGraphs(args[1:], projectRoot, stdout, stderr)
	case "subject-timeline":
		return runMemorySubjectTimeline(args[1:], projectRoot, stdout, stderr)
	case "subject-horizon":
		return runMemorySubjectHorizon(args[1:], projectRoot, stdout, stderr)
	case "subject-experience":
		return runMemorySubjectExperience(args[1:], projectRoot, stdout, stderr)
	case "jobs":
		return runMemoryJobs(args[1:], projectRoot, stdout, stderr)
	case "posterior-run":
		return runMemoryPosteriorRun(args[1:], projectRoot, stdout, stderr)
	case "organize-run":
		return runMemoryOrganizeRun(args[1:], projectRoot, stdout, stderr)
	case "organized":
		return runMemoryOrganized(args[1:], projectRoot, stdout, stderr)
	case "global-organize-run":
		return runMemoryGlobalOrganizeRun(args[1:], projectRoot, stdout, stderr)
	case "global-organized":
		return runMemoryGlobalOrganized(args[1:], projectRoot, stdout, stderr)
	case "global-synthesis-run":
		return runMemoryGlobalSynthesisOrganizeRun(args[1:], projectRoot, stdout, stderr)
	case "global-synthesis":
		return runMemoryGlobalSynthesisOrganized(args[1:], projectRoot, stdout, stderr)
	case "global-card":
		return runMemoryGlobalCard(args[1:], projectRoot, stdout, stderr)
	case "global-synthesis-card":
		return runMemoryGlobalSynthesisCard(args[1:], projectRoot, stdout, stderr)
	case "global-compare":
		return runMemoryGlobalCompare(args[1:], projectRoot, stdout, stderr)
	case "event-graphs":
		return runMemoryEventGraphs(args[1:], projectRoot, stdout, stderr)
	case "event-evidence":
		return runMemoryEventEvidence(args[1:], projectRoot, stdout, stderr)
	case "paradigms":
		return runMemoryParadigms(args[1:], projectRoot, stdout, stderr)
	case "paradigm-evidence":
		return runMemoryParadigmEvidence(args[1:], projectRoot, stdout, stderr)
	case "project-all":
		return runMemoryProjectAll(args[1:], projectRoot, stdout, stderr)
	case "projection-sweep":
		return runMemoryProjectionSweep(args[1:], projectRoot, stdout, stderr)
	case "backfill":
		return runMemoryBackfill(args[1:], projectRoot, stdout, stderr)
	case "cleanup-stale":
		return runMemoryCleanupStale(args[1:], projectRoot, stdout, stderr)
	case "canonical-entities":
		return runMemoryCanonicalEntities(args[1:], projectRoot, stdout, stderr)
	case "canonical-entity-upsert":
		return runMemoryCanonicalEntityUpsert(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, memoryCommandUsage)
		return 2
	}
}
