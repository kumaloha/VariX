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
	factStatusByNode := factStatusMap(record)
	predictionStatusByNode := predictionStatusMap(record)

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
	dedupeGroups := buildDedupeGroups(active, factStatusByNode, predictionStatusByNode)
	contradictionGroups := buildContradictionGroups(active)
	hierarchy := buildHierarchy(active, record)

	output := memory.OrganizationOutput{
		JobID:               job.JobID,
		UserID:              job.UserID,
		SourcePlatform:      job.SourcePlatform,
		SourceExternalID:    job.SourceExternalID,
		GeneratedAt:         now,
		ActiveNodes:         active,
		InactiveNodes:       inactive,
		DedupeGroups:        dedupeGroups,
		ContradictionGroups: contradictionGroups,
		Hierarchy:           hierarchy,
		PredictionStatuses:  extractPredictionStatuses(nodes, record),
		FactVerifications:   extractFactVerifications(active, record),
		OpenQuestions:       buildOpenQuestions(active, record),
		NodeHints:           buildNodeHints(nodes, active, dedupeGroups, contradictionGroups, hierarchy, factStatusByNode, predictionStatusByNode),
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

func buildDedupeGroups(nodes []memory.AcceptedNode, factStatusByNode map[string]compile.FactStatus, predictionStatusByNode map[string]compile.PredictionStatus) []memory.DedupeGroup {
	byText := map[string][]memory.AcceptedNode{}
	for _, node := range nodes {
		key := canonicalNodeText(node.NodeText)
		byText[key] = append(byText[key], node)
	}
	keys := make([]string, 0, len(byText))
	for key := range byText {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]memory.DedupeGroup, 0)
	for _, key := range keys {
		groupNodes := byText[key]
		if len(groupNodes) <= 1 {
			continue
		}
		sort.SliceStable(groupNodes, func(i, j int) bool {
			return compareNodesForDisplay(groupNodes[i], groupNodes[j], factStatusByNode, predictionStatusByNode)
		})
		ids := make([]string, 0, len(groupNodes))
		for _, node := range groupNodes {
			ids = append(ids, node.NodeID)
		}
		out = append(out, memory.DedupeGroup{
			NodeIDs:              ids,
			CanonicalText:        key,
			RepresentativeNodeID: ids[0],
		})
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
			code, reason, ok := contradictionReason(nodes[i].NodeText, nodes[j].NodeText)
			if ok {
				out = append(out, memory.ContradictionGroup{
					NodeIDs:    []string{nodes[i].NodeID, nodes[j].NodeID},
					Reason:     reason,
					ReasonCode: code,
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
			Source:       "graph",
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
					Source:       "inferred",
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

func buildNodeHints(nodes, active []memory.AcceptedNode, dedupeGroups []memory.DedupeGroup, contradictionGroups []memory.ContradictionGroup, hierarchy []memory.HierarchyLink, factStatusByNode map[string]compile.FactStatus, predictionStatusByNode map[string]compile.PredictionStatus) []memory.NodeHint {
	activeSet := map[string]struct{}{}
	for _, node := range active {
		activeSet[node.NodeID] = struct{}{}
	}
	preferredSet := map[string]struct{}{}
	dedupePeers := map[string]map[string]struct{}{}
	for _, group := range dedupeGroups {
		if strings.TrimSpace(group.RepresentativeNodeID) != "" {
			preferredSet[group.RepresentativeNodeID] = struct{}{}
		}
		for _, nodeID := range group.NodeIDs {
			peerSet := ensureNodeSet(dedupePeers, nodeID)
			for _, peerID := range group.NodeIDs {
				if peerID == nodeID {
					continue
				}
				peerSet[peerID] = struct{}{}
			}
		}
	}
	contradictionPeers := map[string]map[string]struct{}{}
	for _, group := range contradictionGroups {
		for _, nodeID := range group.NodeIDs {
			peerSet := ensureNodeSet(contradictionPeers, nodeID)
			for _, peerID := range group.NodeIDs {
				if peerID == nodeID {
					continue
				}
				peerSet[peerID] = struct{}{}
			}
		}
	}
	parentIDs := map[string]map[string]struct{}{}
	childIDs := map[string]map[string]struct{}{}
	for _, link := range hierarchy {
		ensureNodeSet(parentIDs, link.ChildNodeID)[link.ParentNodeID] = struct{}{}
		ensureNodeSet(childIDs, link.ParentNodeID)[link.ChildNodeID] = struct{}{}
	}
	out := make([]memory.NodeHint, 0, len(nodes))
	for _, node := range nodes {
		hint := memory.NodeHint{NodeID: node.NodeID}
		if _, ok := activeSet[node.NodeID]; ok {
			hint.State = "active"
		} else {
			hint.State = "inactive"
		}
		if _, ok := preferredSet[node.NodeID]; ok {
			hint.PreferredForDisplay = true
		}
		if status, ok := factStatusByNode[node.NodeID]; ok {
			hint.VerificationStatus = string(status)
		}
		if status, ok := predictionStatusByNode[node.NodeID]; ok {
			hint.PredictionStatus = string(status)
		}
		hint.DedupePeerNodeIDs = sortedNodeSet(dedupePeers[node.NodeID])
		hint.ContradictionNodeIDs = sortedNodeSet(contradictionPeers[node.NodeID])
		hint.ParentNodeIDs = sortedNodeSet(parentIDs[node.NodeID])
		hint.ChildNodeIDs = sortedNodeSet(childIDs[node.NodeID])
		if hint.State == "active" {
			switch {
			case len(hint.ParentNodeIDs) == 0 && len(hint.ChildNodeIDs) > 0:
				hint.HierarchyRole = "root"
			case len(hint.ParentNodeIDs) > 0 && len(hint.ChildNodeIDs) == 0:
				hint.HierarchyRole = "leaf"
			case len(hint.ParentNodeIDs) > 0 && len(hint.ChildNodeIDs) > 0:
				hint.HierarchyRole = "bridge"
			default:
				hint.HierarchyRole = "isolated"
			}
		}
		out = append(out, hint)
	}
	return out
}

func normalizeNodeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func canonicalNodeText(text string) string {
	normalized := normalizeNodeText(text)
	replacer := strings.NewReplacer(
		"，", "",
		"。", "",
		"！", "",
		"？", "",
		"!", "",
		"?", "",
		".", "",
		",", "",
		"：", "",
		"；", "",
		"、", "",
		"“", "",
		"”", "",
		"（", "",
		"）", "",
		"(", "",
		")", "",
		"继续", "",
		"仍", "",
		"预计", "",
		"预期", "",
		"可能", "",
		"有望", "",
		"正在", "",
		"会", "",
		"将", "",
		"将会", "",
		"走高", "上升",
		"上涨", "上升",
		"攀升", "上升",
		"回升", "上升",
		"下滑", "下降",
		"下跌", "下降",
		"走低", "下降",
		"回落", "下降",
		"上行", "上升",
		"下行", "下降",
		"走弱", "下降",
		"走强", "上升",
		"减弱", "削弱",
		"强化", "增强",
		"支撑", "支持",
	)
	return replacer.Replace(normalized)
}

func contradictionReason(a, b string) (string, string, bool) {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if strings.ReplaceAll(a, "不", "") == b || strings.ReplaceAll(b, "不", "") == a {
		return "negation", "negation-like contradiction", true
	}
	if strings.ReplaceAll(a, "不会", "") == b || strings.ReplaceAll(b, "不会", "") == a {
		return "negation", "negation-like contradiction", true
	}
	for _, pair := range [][2]string{
		{"上升", "下降"},
		{"增加", "减少"},
		{"恶化", "改善"},
		{"紧张", "缓和"},
		{"收缩", "扩张"},
		{"宽松", "收紧"},
		{"削弱", "增强"},
		{"利多", "利空"},
		{"支持", "压制"},
		{"升温", "降温"},
	} {
		if strings.ReplaceAll(a, pair[0], pair[1]) == b || strings.ReplaceAll(a, pair[1], pair[0]) == b {
			return "antonym", "antonym-like contradiction", true
		}
		if strings.ReplaceAll(b, pair[0], pair[1]) == a || strings.ReplaceAll(b, pair[1], pair[0]) == a {
			return "antonym", "antonym-like contradiction", true
		}
	}
	return "", "", false
}

func factStatusMap(record compile.Record) map[string]compile.FactStatus {
	out := make(map[string]compile.FactStatus, len(record.Output.Verification.FactChecks))
	for _, check := range record.Output.Verification.FactChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func predictionStatusMap(record compile.Record) map[string]compile.PredictionStatus {
	out := make(map[string]compile.PredictionStatus, len(record.Output.Verification.PredictionChecks))
	for _, check := range record.Output.Verification.PredictionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func compareNodesForDisplay(a, b memory.AcceptedNode, factStatusByNode map[string]compile.FactStatus, predictionStatusByNode map[string]compile.PredictionStatus) bool {
	scoreA := displayScore(a, factStatusByNode, predictionStatusByNode)
	scoreB := displayScore(b, factStatusByNode, predictionStatusByNode)
	if scoreA != scoreB {
		return scoreA > scoreB
	}
	lenA := len([]rune(canonicalNodeText(a.NodeText)))
	lenB := len([]rune(canonicalNodeText(b.NodeText)))
	if lenA != lenB {
		return lenA < lenB
	}
	if !a.AcceptedAt.Equal(b.AcceptedAt) {
		return a.AcceptedAt.Before(b.AcceptedAt)
	}
	return a.NodeID < b.NodeID
}

func displayScore(node memory.AcceptedNode, factStatusByNode map[string]compile.FactStatus, predictionStatusByNode map[string]compile.PredictionStatus) int {
	score := 0
	switch factStatusByNode[node.NodeID] {
	case compile.FactStatusClearlyTrue:
		score += 40
	case compile.FactStatusUnverifiable:
		score -= 10
	case compile.FactStatusClearlyFalse:
		score -= 20
	}
	switch predictionStatusByNode[node.NodeID] {
	case compile.PredictionStatusResolvedTrue, compile.PredictionStatusResolvedFalse:
		score += 20
	case compile.PredictionStatusUnresolved:
		score += 5
	case compile.PredictionStatusStaleUnresolved:
		score -= 5
	}
	score -= nodeKindRank(node.NodeKind) * 2
	return score
}

func ensureNodeSet(byNode map[string]map[string]struct{}, nodeID string) map[string]struct{} {
	set, ok := byNode[nodeID]
	if !ok {
		set = map[string]struct{}{}
		byNode[nodeID] = set
	}
	return set
}

func sortedNodeSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for nodeID := range set {
		out = append(out, nodeID)
	}
	sort.Strings(out)
	return out
}
