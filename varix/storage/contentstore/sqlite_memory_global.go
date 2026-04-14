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

func (s *SQLiteStore) RunGlobalMemoryOrganization(ctx context.Context, userID string, now time.Time) (memory.GlobalOrganizationOutput, error) {
	if strings.TrimSpace(userID) == "" {
		return memory.GlobalOrganizationOutput{}, fmt.Errorf("user id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	nodes, err := s.ListUserMemory(ctx, strings.TrimSpace(userID))
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}

	globalNodes := make([]memory.AcceptedNode, 0, len(nodes))
	for _, node := range nodes {
		node.NodeID = globalMemoryNodeRef(node)
		globalNodes = append(globalNodes, node)
	}

	active := make([]memory.AcceptedNode, 0, len(globalNodes))
	inactive := make([]memory.AcceptedNode, 0, len(globalNodes))
	for _, node := range globalNodes {
		if isAcceptedNodeActiveAt(node, now) {
			active = append(active, node)
		} else {
			inactive = append(inactive, node)
		}
	}

	dedupeGroups := buildDedupeGroups(active, nil, nil)
	contradictionGroups := buildContradictionGroups(active)
	clusters := buildGlobalClusters(active, dedupeGroups, contradictionGroups, now)
	openQuestions := buildGlobalOpenQuestions(clusters)

	output := memory.GlobalOrganizationOutput{
		UserID:              strings.TrimSpace(userID),
		GeneratedAt:         now,
		ActiveNodes:         active,
		InactiveNodes:       inactive,
		DedupeGroups:        dedupeGroups,
		ContradictionGroups: contradictionGroups,
		Clusters:            clusters,
		OpenQuestions:       openQuestions,
	}

	payload, err := json.Marshal(output)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO global_memory_organization_outputs(user_id, payload_json, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET payload_json = excluded.payload_json, created_at = excluded.created_at`,
		output.UserID, string(payload), now.Format(time.RFC3339Nano))
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	outputID, _ := res.LastInsertId()
	if outputID == 0 {
		_ = s.db.QueryRowContext(ctx, `SELECT output_id FROM global_memory_organization_outputs WHERE user_id = ?`, output.UserID).Scan(&outputID)
	}
	output.OutputID = outputID
	payload, err = json.Marshal(output)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE global_memory_organization_outputs SET payload_json = ?, created_at = ? WHERE user_id = ?`,
		string(payload), now.Format(time.RFC3339Nano), output.UserID); err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	return output, nil
}

func (s *SQLiteStore) GetLatestGlobalMemoryOrganizationOutput(ctx context.Context, userID string) (memory.GlobalOrganizationOutput, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM global_memory_organization_outputs
		 WHERE user_id = ?
		 ORDER BY created_at DESC, output_id DESC
		 LIMIT 1`,
		strings.TrimSpace(userID),
	).Scan(&payload)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	var out memory.GlobalOrganizationOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	return out, nil
}

func buildGlobalClusters(nodes []memory.AcceptedNode, dedupeGroups []memory.DedupeGroup, contradictionGroups []memory.ContradictionGroup, now time.Time) []memory.GlobalCluster {
	byID := map[string]memory.AcceptedNode{}
	for _, node := range nodes {
		byID[node.NodeID] = node
	}

	adj := map[string]map[string]struct{}{}
	addEdge := func(a, b string) {
		if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" || a == b {
			return
		}
		if adj[a] == nil {
			adj[a] = map[string]struct{}{}
		}
		if adj[b] == nil {
			adj[b] = map[string]struct{}{}
		}
		adj[a][b] = struct{}{}
		adj[b][a] = struct{}{}
	}
	for _, group := range dedupeGroups {
		for i := 0; i < len(group.NodeIDs); i++ {
			for j := i + 1; j < len(group.NodeIDs); j++ {
				addEdge(group.NodeIDs[i], group.NodeIDs[j])
			}
		}
	}
	for _, group := range contradictionGroups {
		for i := 0; i < len(group.NodeIDs); i++ {
			for j := i + 1; j < len(group.NodeIDs); j++ {
				addEdge(group.NodeIDs[i], group.NodeIDs[j])
			}
		}
	}

	seen := map[string]struct{}{}
	var clusters []memory.GlobalCluster
	nodeIDs := make([]string, 0, len(byID))
	for id := range byID {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, start := range nodeIDs {
		if _, ok := seen[start]; ok {
			continue
		}
		component := collectComponent(start, adj, seen)
		sort.Strings(component)
		cluster := buildGlobalCluster(component, byID, contradictionGroups, now)
		clusters = append(clusters, cluster)
	}

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].CanonicalProposition < clusters[j].CanonicalProposition
	})
	return clusters
}

func globalMemoryNodeRef(node memory.AcceptedNode) string {
	return node.SourcePlatform + ":" + node.SourceExternalID + ":" + node.NodeID
}

func collectComponent(start string, adj map[string]map[string]struct{}, seen map[string]struct{}) []string {
	queue := []string{start}
	component := make([]string, 0, 4)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		component = append(component, current)
		for next := range adj[current] {
			if _, ok := seen[next]; !ok {
				queue = append(queue, next)
			}
		}
	}
	return component
}

func buildGlobalCluster(component []string, byID map[string]memory.AcceptedNode, contradictionGroups []memory.ContradictionGroup, now time.Time) memory.GlobalCluster {
	supporting := make([]string, 0)
	conflictingSet := map[string]struct{}{}
	conditional := make([]string, 0)
	predictive := make([]string, 0)

	for _, group := range contradictionGroups {
		if overlap(component, group.NodeIDs) {
			for _, id := range group.NodeIDs {
				conflictingSet[id] = struct{}{}
			}
		}
	}

	for _, id := range component {
		node := byID[id]
		switch node.NodeKind {
		case string(compile.NodeExplicitCondition), string(compile.NodeImplicitCondition):
			conditional = append(conditional, id)
		case string(compile.NodePrediction):
			predictive = append(predictive, id)
		default:
			if _, conflicting := conflictingSet[id]; !conflicting {
				supporting = append(supporting, id)
			}
		}
	}

	conflicting := make([]string, 0, len(conflictingSet))
	for id := range conflictingSet {
		conflicting = append(conflicting, id)
	}
	sort.Strings(supporting)
	sort.Strings(conflicting)
	sort.Strings(conditional)
	sort.Strings(predictive)

	rep := chooseRepresentativeNode(component, byID)
	return memory.GlobalCluster{
		ClusterID:            "cluster:" + strings.Join(component, "|"),
		CanonicalProposition: buildCanonicalProposition(component, byID),
		Summary:              buildClusterSummary(component, byID),
		RepresentativeNodeID: rep,
		SupportingNodeIDs:    supporting,
		ConflictingNodeIDs:   conflicting,
		ConditionalNodeIDs:   conditional,
		PredictiveNodeIDs:    predictive,
		Active:               true,
		UpdatedAt:            now,
	}
}

func chooseRepresentativeNode(component []string, byID map[string]memory.AcceptedNode) string {
	bestID := ""
	bestRank := 999
	bestLen := -1
	for _, id := range component {
		node := byID[id]
		rank := representativeRank(node.NodeKind)
		length := len([]rune(strings.TrimSpace(node.NodeText)))
		if rank < bestRank || (rank == bestRank && length > bestLen) || (rank == bestRank && length == bestLen && id < bestID) {
			bestID = id
			bestRank = rank
			bestLen = length
		}
	}
	return bestID
}

func representativeRank(kind string) int {
	switch kind {
	case string(compile.NodeConclusion):
		return 0
	case string(compile.NodeFact):
		return 1
	case string(compile.NodeImplicitCondition):
		return 2
	case string(compile.NodeExplicitCondition):
		return 3
	case string(compile.NodePrediction):
		return 4
	default:
		return 99
	}
}

func buildCanonicalProposition(component []string, byID map[string]memory.AcceptedNode) string {
	if proposition, ok := deriveNeutralProposition(component, byID); ok {
		return proposition
	}
	repID := chooseRepresentativeNode(component, byID)
	text := strings.TrimSpace(byID[repID].NodeText)
	if text == "" {
		return "未命名认知簇"
	}
	text = strings.TrimPrefix(text, "若")
	text = strings.TrimPrefix(text, "如果")
	text = strings.TrimPrefix(text, "一旦")
	text = strings.TrimPrefix(text, "假如")
	text = strings.TrimSpace(text)
	return truncateText(text, 80)
}

func deriveNeutralProposition(component []string, byID map[string]memory.AcceptedNode) (string, bool) {
	if len(component) < 2 {
		return "", false
	}
	var canonicals []string
	for _, id := range component {
		text := strings.TrimSpace(byID[id].NodeText)
		if text == "" {
			continue
		}
		text = strings.TrimPrefix(text, "若")
		text = strings.TrimPrefix(text, "如果")
		text = strings.TrimPrefix(text, "一旦")
		text = strings.TrimPrefix(text, "假如")
		text = strings.TrimSpace(text)
		canonical := canonicalNodeText(text)
		if canonical != "" {
			canonicals = append(canonicals, canonical)
		}
	}
	if len(canonicals) < 2 {
		return "", false
	}
	prefix := commonPrefix(canonicals)
	prefix = strings.TrimSpace(prefix)
	if len([]rune(prefix)) < 2 {
		return "", false
	}
	return "关于「" + truncateText(prefix, 40) + "」的判断", true
}

func buildClusterSummary(component []string, byID map[string]memory.AcceptedNode) string {
	repID := chooseRepresentativeNode(component, byID)
	return truncateText(strings.TrimSpace(byID[repID].NodeText), 120)
}

func buildGlobalOpenQuestions(clusters []memory.GlobalCluster) []string {
	out := make([]string, 0)
	for _, cluster := range clusters {
		if len(cluster.ConflictingNodeIDs) > 0 {
			out = append(out, fmt.Sprintf("cluster %s contains unresolved contradictions", cluster.ClusterID))
		}
	}
	sort.Strings(out)
	return out
}

func overlap(component, group []string) bool {
	set := map[string]struct{}{}
	for _, id := range component {
		set[id] = struct{}{}
	}
	for _, id := range group {
		if _, ok := set[id]; ok {
			return true
		}
	}
	return false
}

func truncateText(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func commonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := []rune(values[0])
	for _, value := range values[1:] {
		runes := []rune(value)
		limit := len(prefix)
		if len(runes) < limit {
			limit = len(runes)
		}
		i := 0
		for i < limit && prefix[i] == runes[i] {
			i++
		}
		prefix = prefix[:i]
		if len(prefix) == 0 {
			return ""
		}
	}
	return string(prefix)
}

func listAllUserMemoryTx(ctx context.Context, tx *sql.Tx, userID string) ([]memory.AcceptedNode, error) {
	rows, err := tx.QueryContext(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		FROM user_memory_nodes
		WHERE user_id = ?
		ORDER BY accepted_at ASC, memory_id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryNodes(rows)
}
