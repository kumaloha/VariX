package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func (s *SQLiteStore) RunGlobalMemorySynthesis(ctx context.Context, userID string, now time.Time) (memory.GlobalMemorySynthesisOutput, error) {
	return s.runGlobalMemorySynthesis(ctx, userID, now, true)
}

func (s *SQLiteStore) runGlobalMemorySynthesis(ctx context.Context, userID string, now time.Time, refreshProjections bool) (memory.GlobalMemorySynthesisOutput, error) {
	var err error
	userID, err = normalizeRequiredUserID(userID)
	if err != nil {
		return memory.GlobalMemorySynthesisOutput{}, err
	}
	now = normalizeNow(now)
	if refreshProjections {
		// Refresh graph-first persisted projections first so global-synthesis sees fresh event/paradigm layers.
		if _, err := s.RunEventGraphProjection(ctx, userID, now); err != nil {
			return memory.GlobalMemorySynthesisOutput{}, err
		}
		if _, err := s.RunParadigmProjection(ctx, userID, now); err != nil {
			return memory.GlobalMemorySynthesisOutput{}, err
		}
	}

	nodes, err := s.ListUserMemory(ctx, userID)
	if err != nil {
		return memory.GlobalMemorySynthesisOutput{}, err
	}
	persistedEventGraphs, _ := s.ListEventGraphs(ctx, userID)
	persistedParadigms, _ := s.ListParadigms(ctx, userID)
	activeNodes, nodesByID := activeGlobalAcceptedNodeIndex(nodes, now)
	candidateTheses := buildCandidateTheses(activeNodes, now)
	conflicts := make([]memory.ConflictSet, 0)
	causalTheses := make([]memory.CausalThesis, 0)
	cognitiveCards := make([]memory.CognitiveCard, 0)
	cognitiveConclusions := make([]memory.CognitiveConclusion, 0)
	for _, thesis := range candidateTheses {
		result := detectThesisConflict(thesis, nodesByID, now)
		if result.Blocked && result.Conflict != nil {
			conflict := *result.Conflict
			enrichConflictSetFromCompiledOutput(ctx, s, &conflict)
			conflicts = append(conflicts, conflict)
			continue
		}
		causal := buildCausalThesis(thesis, nodesByID)
		causalTheses = append(causalTheses, causal)
		cards := buildCognitiveCards(causal, nodesByID)
		cognitiveCards = append(cognitiveCards, cards...)
		if conclusion, ok := buildCognitiveConclusion(causal, cards); ok {
			cognitiveConclusions = append(cognitiveConclusions, conclusion)
		}
	}
	topMemoryItems := buildTopMemoryItems(conflicts, cognitiveConclusions, cognitiveCards, now)
	relationProjection, err := s.buildRelationFirstProjection(ctx, now, causalTheses, cognitiveConclusions)
	if err != nil {
		return memory.GlobalMemorySynthesisOutput{}, err
	}

	applyGlobalSynthesisFallbacks(&relationProjection, persistedEventGraphs, persistedParadigms, now, &cognitiveCards, &cognitiveConclusions, &topMemoryItems)

	output := memory.GlobalMemorySynthesisOutput{
		UserID:               userID,
		GeneratedAt:          now,
		CanonicalEntities:    relationProjection.canonicalEntities,
		Relations:            relationProjection.relations,
		Mechanisms:           relationProjection.mechanisms,
		MechanismNodes:       relationProjection.mechanismNodes,
		MechanismEdges:       relationProjection.mechanismEdges,
		PathOutcomes:         relationProjection.pathOutcomes,
		DriverAggregates:     relationProjection.driverAggregates,
		TargetAggregates:     relationProjection.targetAggregates,
		ConflictViews:        relationProjection.conflictViews,
		CandidateTheses:      candidateTheses,
		ConflictSets:         conflicts,
		CausalTheses:         causalTheses,
		CognitiveCards:       cognitiveCards,
		CognitiveConclusions: cognitiveConclusions,
		TopMemoryItems:       topMemoryItems,
	}
	return persistGlobalMemorySynthesisOutput(ctx, s.db, output)
}

func applyGlobalSynthesisFallbacks(
	relationProjection *relationFirstProjection,
	persistedEventGraphs []EventGraphRecord,
	persistedParadigms []ParadigmRecord,
	now time.Time,
	cognitiveCards *[]memory.CognitiveCard,
	cognitiveConclusions *[]memory.CognitiveConclusion,
	topMemoryItems *[]memory.TopMemoryItem,
) {
	if relationProjection == nil {
		return
	}
	if len(relationProjection.driverAggregates) == 0 && len(persistedEventGraphs) > 0 {
		relationProjection.driverAggregates = buildDriverAggregatesFromEventGraphs(persistedEventGraphs, now)
	}
	if len(relationProjection.targetAggregates) == 0 && len(persistedEventGraphs) > 0 {
		relationProjection.targetAggregates = buildTargetAggregatesFromEventGraphs(persistedEventGraphs, now)
	}
	if len(*cognitiveCards) == 0 && len(persistedEventGraphs) > 0 {
		*cognitiveCards = buildCardsFromEventGraphs(persistedEventGraphs, now)
	}
	if len(*cognitiveConclusions) == 0 && len(persistedParadigms) > 0 {
		*cognitiveConclusions = buildConclusionsFromParadigms(persistedParadigms, now)
	}
	if len(*topMemoryItems) == 0 && len(persistedParadigms) > 0 {
		*topMemoryItems = buildTopItemsFromParadigms(persistedParadigms, now)
	}
	if len(*topMemoryItems) == 0 && len(persistedEventGraphs) > 0 {
		*topMemoryItems = buildTopItemsFromEventGraphs(persistedEventGraphs, now)
	}
}

func (s *SQLiteStore) GetLatestGlobalMemorySynthesisOutput(ctx context.Context, userID string) (memory.GlobalMemorySynthesisOutput, error) {
	payload, err := latestUserScopedPayload(ctx, s.db, "global_memory_synthesis_outputs", userID)
	if err != nil {
		return memory.GlobalMemorySynthesisOutput{}, err
	}
	var out memory.GlobalMemorySynthesisOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.GlobalMemorySynthesisOutput{}, err
	}
	return out, nil
}

func persistGlobalMemorySynthesisOutput(ctx context.Context, db *sql.DB, output memory.GlobalMemorySynthesisOutput) (memory.GlobalMemorySynthesisOutput, error) {
	if err := persistLatestUserScopedOutput(ctx, db, "global_memory_synthesis_outputs", output.UserID, output.GeneratedAt, &output, func(id int64) {
		output.OutputID = id
	}); err != nil {
		return memory.GlobalMemorySynthesisOutput{}, err
	}
	return output, nil
}

func enrichConflictSetFromCompiledOutput(ctx context.Context, store *SQLiteStore, conflict *memory.ConflictSet) {
	if conflict == nil {
		return
	}
	if why, refs := compiledConflictWhy(ctx, store, conflict.SideANodeIDs); len(why) > 0 {
		conflict.SideAWhy = why
		if len(refs) > 0 {
			conflict.SideASourceRefs = refs
		}
	}
	if why, refs := compiledConflictWhy(ctx, store, conflict.SideBNodeIDs); len(why) > 0 {
		conflict.SideBWhy = why
		if len(refs) > 0 {
			conflict.SideBSourceRefs = refs
		}
	}
}

func compiledConflictWhy(ctx context.Context, store *SQLiteStore, globalNodeIDs []string) ([]string, []string) {
	if len(globalNodeIDs) == 0 {
		return nil, nil
	}
	platform, externalID, localNodeID, ok := splitGlobalNodeRef(globalNodeIDs[0])
	if !ok {
		return nil, nil
	}
	record, err := store.GetCompiledOutput(ctx, platform, externalID)
	if err != nil {
		return nil, nil
	}
	nodeByID := map[string]model.GraphNode{}
	for _, node := range record.Output.Graph.Nodes {
		nodeByID[node.ID] = node
	}
	type supportNode struct {
		text     string
		priority int
		depth    int
		order    int
	}
	supports := make([]supportNode, 0)
	appendSupport := func(node model.GraphNode, depth int) {
		if text := strings.TrimSpace(node.Text); text != "" {
			supports = append(supports, supportNode{
				text:     text,
				priority: conflictSupportPriority(node.Kind),
				depth:    depth,
				order:    len(supports),
			})
		}
	}
	directSupportIDs := make([]string, 0)
	for _, edge := range record.Output.Graph.Edges {
		if edge.To != localNodeID {
			continue
		}
		node, ok := nodeByID[edge.From]
		if !ok {
			continue
		}
		switch node.Kind {
		case model.NodeFact, model.NodeExplicitCondition, model.NodeImplicitCondition:
			appendSupport(node, 0)
			directSupportIDs = append(directSupportIDs, node.ID)
		}
	}
	for _, supportID := range directSupportIDs {
		for _, edge := range record.Output.Graph.Edges {
			if edge.To != supportID {
				continue
			}
			node, ok := nodeByID[edge.From]
			if !ok {
				continue
			}
			switch node.Kind {
			case model.NodeFact, model.NodeExplicitCondition, model.NodeImplicitCondition:
				appendSupport(node, 1)
			}
		}
	}
	if len(supports) == 0 {
		return nil, nil
	}
	sort.SliceStable(supports, func(i, j int) bool {
		if supports[i].depth != supports[j].depth {
			return supports[i].depth < supports[j].depth
		}
		if supports[i].priority != supports[j].priority {
			return supports[i].priority < supports[j].priority
		}
		return supports[i].order < supports[j].order
	})
	why := make([]string, 0, len(supports))
	for _, support := range supports {
		why = append(why, support.text)
	}
	return uniquePreservingOrder(why), []string{platform + ":" + externalID}
}

func splitGlobalNodeRef(ref string) (platform, externalID, localNodeID string, ok bool) {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func conflictSupportPriority(kind model.NodeKind) int {
	switch kind {
	case model.NodeFact:
		return 0
	case model.NodeExplicitCondition:
		return 1
	case model.NodeImplicitCondition:
		return 2
	default:
		return 99
	}
}

func buildDriverAggregatesFromEventGraphs(graphs []EventGraphRecord, now time.Time) []memory.DriverAggregate {
	out := make([]memory.DriverAggregate, 0)
	forEachEventGraphScope(graphs, "driver", func(graph EventGraphRecord) {
		out = append(out, memory.DriverAggregate{
			AggregateID:        graph.EventGraphID,
			DriverEntityID:     relationEntityID(memory.CanonicalEntityDriver, graph.AnchorSubject),
			TargetEntityIDs:    nil,
			CoverageScore:      float64(graph.PrimaryNodeCount),
			ConflictCount:      0,
			TraceabilityStatus: memory.TraceabilityPartial,
			AsOf:               now,
			CreatedAt:          now,
		})
	})
	return out
}

func buildTargetAggregatesFromEventGraphs(graphs []EventGraphRecord, now time.Time) []memory.TargetAggregate {
	out := make([]memory.TargetAggregate, 0)
	forEachEventGraphScope(graphs, "target", func(graph EventGraphRecord) {
		out = append(out, memory.TargetAggregate{
			AggregateID:        graph.EventGraphID,
			TargetEntityID:     relationEntityID(memory.CanonicalEntityTarget, graph.AnchorSubject),
			DriverEntityIDs:    nil,
			CoverageScore:      float64(graph.PrimaryNodeCount),
			ConflictCount:      0,
			TraceabilityStatus: memory.TraceabilityPartial,
			AsOf:               now,
			CreatedAt:          now,
		})
	})
	return out
}

func buildConclusionsFromParadigms(items []ParadigmRecord, now time.Time) []memory.CognitiveConclusion {
	out := make([]memory.CognitiveConclusion, 0, len(items))
	for _, item := range items {
		out = append(out, newCognitiveConclusion(
			item.ParadigmID+"-conclusion",
			"paradigm",
			item.ParadigmID,
			paradigmHeadline(item),
			buildCardOrConclusionSubheadline(item.CredibilityState),
			memory.TraceabilityPartial,
			now,
			now,
		))
	}
	return out
}

func buildTopItemsFromParadigms(items []ParadigmRecord, now time.Time) []memory.TopMemoryItem {
	out := make([]memory.TopMemoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, newTopMemoryItemWithAsOf(
			item.ParadigmID+"-top",
			memory.TopMemoryItemConclusion,
			paradigmHeadline(item),
			"",
			item.ParadigmID,
			memory.SignalMedium,
			now,
			now,
		))
	}
	return out
}

func buildCardsFromEventGraphs(graphs []EventGraphRecord, now time.Time) []memory.CognitiveCard {
	out := make([]memory.CognitiveCard, 0, len(graphs))
	for _, graph := range graphs {
		out = append(out, memory.CognitiveCard{
			CardID:         graph.EventGraphID + "-card",
			RelationID:     graph.EventGraphID,
			AsOf:           now,
			CardType:       "event_graph",
			Title:          graph.AnchorSubject,
			Summary:        eventGraphSummary(graph),
			MechanismChain: cloneStringSlice(graph.RepresentativeChanges),
			SourceRefs:     cloneStringSlice(graph.SourceSubgraphIDs),
			CreatedAt:      now,
		})
	}
	return out
}

func buildTopItemsFromEventGraphs(graphs []EventGraphRecord, now time.Time) []memory.TopMemoryItem {
	out := make([]memory.TopMemoryItem, 0, len(graphs))
	for _, graph := range graphs {
		out = append(out, newTopMemoryItemWithAsOf(
			graph.EventGraphID+"-top",
			memory.TopMemoryItemCard,
			graph.AnchorSubject,
			eventGraphSummary(graph),
			graph.EventGraphID,
			memory.SignalMedium,
			now,
			now,
		))
	}
	return out
}

func eventGraphSummary(graph EventGraphRecord) string {
	return graph.Scope + " / " + graph.TimeBucket
}

func paradigmHeadline(item ParadigmRecord) string {
	return item.DriverSubject + " -> " + item.TargetSubject
}

func forEachEventGraphScope(graphs []EventGraphRecord, scope string, fn func(EventGraphRecord)) {
	for _, graph := range graphs {
		if graph.Scope != scope {
			continue
		}
		fn(graph)
	}
}

func activeGlobalAcceptedNodeIndex(nodes []memory.AcceptedNode, now time.Time) ([]memory.AcceptedNode, map[string]memory.AcceptedNode) {
	activeNodes := make([]memory.AcceptedNode, 0, len(nodes))
	nodesByID := make(map[string]memory.AcceptedNode, len(nodes))
	for _, node := range nodes {
		if !isAcceptedNodeActiveAt(node, now) {
			continue
		}
		activeNodes = append(activeNodes, node)
		globalNode := node
		ref := globalMemoryNodeRef(globalNode)
		globalNode.NodeID = ref
		nodesByID[ref] = globalNode
	}
	return activeNodes, nodesByID
}
