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

func (s *SQLiteStore) RunNextMemoryOrganizationJob(ctx context.Context, userID string, now time.Time) (memory.OrganizationOutput, error) {
	var job memory.OrganizationJob
	var createdAt string
	query := `SELECT job_id, trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at
		FROM memory_organization_jobs
		WHERE status = 'queued'`
	args := []any{}
	if strings.TrimSpace(userID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(userID))
	}
	query += ` ORDER BY created_at ASC, job_id ASC LIMIT 1`
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&job.JobID, &job.TriggerEventID, &job.UserID, &job.SourcePlatform, &job.SourceExternalID, &job.Status, &createdAt, new(sql.NullString), new(sql.NullString))
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	job.CreatedAt = parseSQLiteTime(createdAt)
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_jobs SET status = ?, started_at = ? WHERE job_id = ?`, "running", now.Format(time.RFC3339Nano), job.JobID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	nodes, err := listUserMemoryBySourceTx(ctx, tx, job.UserID, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	record, err := getCompiledOutputTx(ctx, tx, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}

	active := make([]memory.AcceptedNode, 0)
	inactive := make([]memory.AcceptedNode, 0)
	for _, node := range nodes {
		if node.ValidFrom.IsZero() || node.ValidTo.IsZero() {
			inactive = append(inactive, node)
			continue
		}
		if now.Before(node.ValidFrom) || now.After(node.ValidTo) {
			inactive = append(inactive, node)
		} else {
			active = append(active, node)
		}
	}

	output := memory.OrganizationOutput{
		JobID:               job.JobID,
		UserID:              job.UserID,
		SourcePlatform:      job.SourcePlatform,
		SourceExternalID:    job.SourceExternalID,
		GeneratedAt:         now,
		ActiveNodes:         active,
		InactiveNodes:       inactive,
		DedupeGroups:        buildDedupeGroups(active),
		ContradictionGroups: buildContradictionGroups(active),
		Hierarchy:           buildHierarchy(active, record),
		PredictionStatuses:  extractPredictionStatuses(nodes, record),
		FactVerifications:   extractFactVerifications(active, record),
		OpenQuestions:       buildOpenQuestions(active, record),
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO memory_organization_outputs(job_id, user_id, source_platform, source_external_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET created_at = excluded.created_at`,
		job.JobID, job.UserID, job.SourcePlatform, job.SourceExternalID, "{}", now.Format(time.RFC3339Nano))
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	outputID, _ := res.LastInsertId()
	if outputID == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT output_id FROM memory_organization_outputs WHERE job_id = ?`, job.JobID).Scan(&outputID)
	}
	output.OutputID = outputID
	payload, err := json.Marshal(output)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_outputs SET payload_json = ?, created_at = ? WHERE output_id = ?`, string(payload), now.Format(time.RFC3339Nano), outputID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_jobs SET status = ?, finished_at = ? WHERE job_id = ?`, "done", now.Format(time.RFC3339Nano), job.JobID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return memory.OrganizationOutput{}, err
	}
	return output, nil
}

func (s *SQLiteStore) GetLatestMemoryOrganizationOutput(ctx context.Context, userID, sourcePlatform, sourceExternalID string) (memory.OrganizationOutput, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM memory_organization_outputs
		 WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		 ORDER BY created_at DESC, output_id DESC
		 LIMIT 1`,
		userID, sourcePlatform, sourceExternalID,
	).Scan(&payload)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	var out memory.OrganizationOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.OrganizationOutput{}, err
	}
	return out, nil
}

func listUserMemoryBySourceTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID string) ([]memory.AcceptedNode, error) {
	rows, err := tx.QueryContext(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		FROM user_memory_nodes
		WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		ORDER BY accepted_at ASC, memory_id ASC`, userID, sourcePlatform, sourceExternalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryNodes(rows)
}

func getCompiledOutputTx(ctx context.Context, tx *sql.Tx, platform, externalID string) (compile.Record, error) {
	var payload string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM compiled_outputs WHERE platform = ? AND external_id = ?`, platform, externalID).Scan(&payload); err != nil {
		return compile.Record{}, err
	}
	var record compile.Record
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return compile.Record{}, err
	}
	return record, nil
}

func buildDedupeGroups(nodes []memory.AcceptedNode) []memory.DedupeGroup {
	byText := map[string][]string{}
	for _, node := range nodes {
		key := canonicalNodeText(node.NodeText)
		byText[key] = append(byText[key], node.NodeID)
	}
	out := make([]memory.DedupeGroup, 0)
	for _, ids := range byText {
		if len(ids) > 1 {
			out = append(out, memory.DedupeGroup{NodeIDs: ids})
		}
	}
	return out
}

func buildContradictionGroups(nodes []memory.AcceptedNode) []memory.ContradictionGroup {
	out := make([]memory.ContradictionGroup, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if canonicalNodeText(nodes[i].NodeText) == canonicalNodeText(nodes[j].NodeText) {
				continue
			}
			if areContradictory(nodes[i].NodeText, nodes[j].NodeText) {
				out = append(out, memory.ContradictionGroup{
					NodeIDs: []string{nodes[i].NodeID, nodes[j].NodeID},
					Reason:  "negation-like contradiction",
				})
			}
		}
	}
	return out
}

func buildHierarchy(nodes []memory.AcceptedNode, record compile.Record) []memory.HierarchyLink {
	active := map[string]struct{}{}
	for _, node := range nodes {
		active[node.NodeID] = struct{}{}
	}
	factStatusByNode := map[string]compile.FactStatus{}
	for _, check := range record.Output.Verification.FactChecks {
		factStatusByNode[check.NodeID] = check.Status
	}
	out := make([]memory.HierarchyLink, 0)
	seen := map[string]struct{}{}
	for _, edge := range record.Output.Graph.Edges {
		if _, ok := active[edge.From]; !ok {
			continue
		}
		if _, ok := active[edge.To]; !ok {
			continue
		}
		if status, ok := factStatusByNode[edge.From]; ok && status != compile.FactStatusClearlyTrue {
			continue
		}
		link := memory.HierarchyLink{
			ParentNodeID: edge.From,
			ChildNodeID:  edge.To,
			Kind:         string(edge.Kind),
		}
		key := link.ParentNodeID + "->" + link.ChildNodeID
		seen[key] = struct{}{}
		out = append(out, link)
	}

	byRank := map[int][]memory.AcceptedNode{}
	ranks := []int{}
	seenRanks := map[int]struct{}{}
	for _, node := range nodes {
		rank := nodeKindRank(node.NodeKind)
		byRank[rank] = append(byRank[rank], node)
		if _, ok := seenRanks[rank]; !ok {
			seenRanks[rank] = struct{}{}
			ranks = append(ranks, rank)
		}
	}
	sort.Ints(ranks)
	for i := 0; i < len(ranks)-1; i++ {
		lower := byRank[ranks[i]]
		upper := byRank[ranks[i+1]]
		for _, parent := range lower {
			if status, ok := factStatusByNode[parent.NodeID]; ok && status != compile.FactStatusClearlyTrue {
				continue
			}
			for _, child := range upper {
				key := parent.NodeID + "->" + child.NodeID
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, memory.HierarchyLink{
					ParentNodeID: parent.NodeID,
					ChildNodeID:  child.NodeID,
					Kind:         "inferred",
				})
			}
		}
	}
	return out
}

func nodeKindRank(kind string) int {
	switch kind {
	case string(compile.NodeFact):
		return 0
	case string(compile.NodeAssumption):
		return 1
	case string(compile.NodeConclusion):
		return 2
	case string(compile.NodePrediction):
		return 3
	default:
		return 100
	}
}

func extractPredictionStatuses(nodes []memory.AcceptedNode, record compile.Record) []memory.PredictionStatus {
	accepted := map[string]struct{}{}
	for _, node := range nodes {
		accepted[node.NodeID] = struct{}{}
	}
	out := make([]memory.PredictionStatus, 0)
	for _, check := range record.Output.Verification.PredictionChecks {
		if _, ok := accepted[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.PredictionStatus{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	return out
}

func extractFactVerifications(nodes []memory.AcceptedNode, record compile.Record) []memory.FactVerification {
	active := map[string]struct{}{}
	for _, node := range nodes {
		active[node.NodeID] = struct{}{}
	}
	out := make([]memory.FactVerification, 0)
	for _, check := range record.Output.Verification.FactChecks {
		if _, ok := active[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.FactVerification{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	return out
}

func buildOpenQuestions(nodes []memory.AcceptedNode, record compile.Record) []string {
	questions := make([]string, 0)
	for _, node := range nodes {
		if node.ValidFrom.IsZero() || node.ValidTo.IsZero() {
			questions = append(questions, fmt.Sprintf("node %s has no validity window", node.NodeID))
		}
	}
	for _, check := range record.Output.Verification.FactChecks {
		if check.Status == compile.FactStatusUnverifiable {
			questions = append(questions, fmt.Sprintf("fact node %s remains unverifiable", check.NodeID))
		}
	}
	return questions
}

func normalizeNodeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func canonicalNodeText(text string) string {
	normalized := normalizeNodeText(text)
	replacer := strings.NewReplacer(
		"，", "",
		"。", "",
		"：", "",
		"；", "",
		"、", "",
		"“", "",
		"”", "",
		"（", "",
		"）", "",
		"(", "",
		")", "",
		"会", "",
		"将", "",
		"下滑", "下降",
		"上行", "上升",
		"下行", "下降",
		"走弱", "下降",
		"走强", "上升",
	)
	return replacer.Replace(normalized)
}

func areContradictory(a, b string) bool {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if strings.ReplaceAll(a, "不", "") == b || strings.ReplaceAll(b, "不", "") == a {
		return true
	}
	if strings.ReplaceAll(a, "不会", "") == b || strings.ReplaceAll(b, "不会", "") == a {
		return true
	}
	for _, pair := range [][2]string{
		{"上升", "下降"},
		{"增加", "减少"},
		{"恶化", "改善"},
		{"紧张", "缓和"},
		{"收缩", "扩张"},
		{"宽松", "收紧"},
	} {
		if strings.ReplaceAll(a, pair[0], pair[1]) == b || strings.ReplaceAll(a, pair[1], pair[0]) == b {
			return true
		}
		if strings.ReplaceAll(b, pair[0], pair[1]) == a || strings.ReplaceAll(b, pair[1], pair[0]) == a {
			return true
		}
	}
	return false
}
