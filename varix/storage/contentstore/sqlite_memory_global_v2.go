package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func (s *SQLiteStore) RunGlobalMemoryOrganizationV2(ctx context.Context, userID string, now time.Time) (memory.GlobalMemoryV2Output, error) {
	if strings.TrimSpace(userID) == "" {
		return memory.GlobalMemoryV2Output{}, fmt.Errorf("user id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	nodes, err := s.ListUserMemory(ctx, strings.TrimSpace(userID))
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	activeNodes := make([]memory.AcceptedNode, 0, len(nodes))
	nodesByID := make(map[string]memory.AcceptedNode, len(nodes))
	for _, node := range nodes {
		if isAcceptedNodeActiveAt(node, now) {
			activeNodes = append(activeNodes, node)
			ref := globalMemoryNodeRef(node)
			node.NodeID = ref
			nodesByID[ref] = node
		}
	}
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

	output := memory.GlobalMemoryV2Output{
		UserID:               strings.TrimSpace(userID),
		GeneratedAt:          now,
		CandidateTheses:      candidateTheses,
		ConflictSets:         conflicts,
		CausalTheses:         causalTheses,
		CognitiveCards:       cognitiveCards,
		CognitiveConclusions: cognitiveConclusions,
		TopMemoryItems:       topMemoryItems,
	}
	return persistGlobalMemoryV2Output(ctx, s.db, output)
}

func (s *SQLiteStore) GetLatestGlobalMemoryOrganizationV2Output(ctx context.Context, userID string) (memory.GlobalMemoryV2Output, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM global_memory_v2_outputs
		 WHERE user_id = ?
		 ORDER BY created_at DESC, output_id DESC
		 LIMIT 1`,
		strings.TrimSpace(userID),
	).Scan(&payload)
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	var out memory.GlobalMemoryV2Output
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	return out, nil
}

func persistGlobalMemoryV2Output(ctx context.Context, db *sql.DB, output memory.GlobalMemoryV2Output) (memory.GlobalMemoryV2Output, error) {
	payload, err := json.Marshal(output)
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	res, err := db.ExecContext(ctx, `INSERT INTO global_memory_v2_outputs(user_id, payload_json, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET payload_json = excluded.payload_json, created_at = excluded.created_at`,
		output.UserID, string(payload), output.GeneratedAt.Format(time.RFC3339Nano))
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	outputID, _ := res.LastInsertId()
	if outputID == 0 {
		_ = db.QueryRowContext(ctx, `SELECT output_id FROM global_memory_v2_outputs WHERE user_id = ?`, output.UserID).Scan(&outputID)
	}
	output.OutputID = outputID

	payload, err = json.Marshal(output)
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	if _, err := db.ExecContext(ctx, `UPDATE global_memory_v2_outputs SET payload_json = ?, created_at = ? WHERE user_id = ?`,
		string(payload), output.GeneratedAt.Format(time.RFC3339Nano), output.UserID); err != nil {
		return memory.GlobalMemoryV2Output{}, err
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
	nodeByID := map[string]compile.GraphNode{}
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
	appendSupport := func(node compile.GraphNode, depth int) {
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
		case compile.NodeFact, compile.NodeExplicitCondition, compile.NodeImplicitCondition:
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
			case compile.NodeFact, compile.NodeExplicitCondition, compile.NodeImplicitCondition:
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

func conflictSupportPriority(kind compile.NodeKind) int {
	switch kind {
	case compile.NodeFact:
		return 0
	case compile.NodeExplicitCondition:
		return 1
	case compile.NodeImplicitCondition:
		return 2
	default:
		return 99
	}
}
