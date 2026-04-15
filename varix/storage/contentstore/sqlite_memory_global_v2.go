package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
			conflicts = append(conflicts, *result.Conflict)
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
	topMemoryItems := buildTopMemoryItems(conflicts, cognitiveConclusions, now)

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
