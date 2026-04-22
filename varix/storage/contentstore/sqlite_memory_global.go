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
	var err error
	userID, err = normalizeRequiredUserID(userID)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}
	now = normalizeNow(now)

	nodes, err := s.ListUserMemory(ctx, userID)
	if err != nil {
		return memory.GlobalOrganizationOutput{}, err
	}

	globalNodes := globalizeAcceptedNodes(nodes)
	active, inactive := splitAcceptedNodesByActivity(globalNodes, now)

	dedupeGroups := buildDedupeGroups(active, nil, nil)
	contradictionGroups := buildContradictionGroups(active)
	clusters := buildGlobalClusters(active, dedupeGroups, contradictionGroups, now)
	openQuestions := buildGlobalOpenQuestions(clusters)

	output := memory.GlobalOrganizationOutput{
		UserID:              userID,
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
	nodeIDs := make([]string, 0, len(byID))
	for id := range byID {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
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
	for i := 0; i < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			left := byID[nodeIDs[i]]
			right := byID[nodeIDs[j]]
			if theme := sharedMacroTheme(left.NodeText, right.NodeText); theme != "" {
				addEdge(left.NodeID, right.NodeID)
				continue
			}
			if !sameGlobalClusterFamily(left, right) {
				continue
			}
			if nonEmptySemanticPhrase(left.NodeText, right.NodeText) != "" {
				addEdge(left.NodeID, right.NodeID)
			}
		}
	}

	seen := map[string]struct{}{}
	var clusters []memory.GlobalCluster
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

func sameGlobalClusterFamily(left, right memory.AcceptedNode) bool {
	bucket := func(kind string) string {
		switch kind {
		case string(compile.NodeFact), string(compile.NodeConclusion), string(compile.NodePrediction):
			return "claim"
		case string(compile.NodeExplicitCondition), string(compile.NodeImplicitCondition):
			return "condition"
		default:
			return "other"
		}
	}
	return bucket(left.NodeKind) == bucket(right.NodeKind)
}

func sharedSemanticPhrase(a, b string) (string, bool) {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if a == "" || b == "" {
		return "", false
	}
	phrase := longestCommonSubstring([]rune(a), []rune(b))
	phrase = strings.TrimSpace(phrase)
	if len([]rune(phrase)) < 4 {
		return "", false
	}
	for _, banned := range []string{"金融资产", "投资者", "周期", "风险", "资产", "市场", "美国"} {
		if phrase == banned {
			return "", false
		}
	}
	return phrase, true
}

func sharedMacroTheme(a, b string) string {
	left := macroThemeKey(a)
	right := macroThemeKey(b)
	if left == "" || right == "" {
		return ""
	}
	if left == right {
		return left
	}
	return ""
}

func macroThemeKey(text string) string {
	text = canonicalNodeText(text)
	switch {
	case containsAnyText(text, "银行去监管", "去监管", "金融体系安全", "银行体系更安全", "支持经济增长"):
		return "macro-bank-regulation"
	case containsAnyText(text, "石油美元", "油价", "霍尔木兹", "私募信贷", "流动性", "挤兑", "美债", "美股", "华尔街", "伊朗", "中东", "大宗商品", "供应链"):
		return "macro-liquidity"
	case containsAnyText(text, "债务", "金融资产", "货币", "央行", "通胀", "购买力", "回报", "资产价格", "利率", "脆弱性"):
		return "macro-debt"
	case containsAnyText(text, "能源短缺", "生活成本", "k型"):
		return "macro-supply-shock"
	default:
		return ""
	}
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
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
	canonical := buildCanonicalProposition(component, byID)
	if len(conflicting) > 0 && !strings.HasPrefix(canonical, "关于「") {
		if conflictCanonical, ok := deriveContradictionProposition(conflicting, byID); ok {
			canonical = conflictCanonical
		}
	}
	coreSupporting := selectCoreNodes(supporting, byID, 2)
	coreConditional := selectCoreNodes(conditional, byID, 2)
	coreConclusions := selectCoreNodes(filterNodesByKind(component, byID, string(compile.NodeConclusion)), byID, 1)
	corePredictive := selectCoreNodes(predictive, byID, 2)
	expanded := cloneStringSlice(component)
	sort.Strings(expanded)
	return memory.GlobalCluster{
		ClusterID:              "cluster:" + strings.Join(component, "|"),
		CanonicalProposition:   canonical,
		Summary:                buildClusterSummary(canonical, supporting, conflicting, conditional, predictive, byID),
		RepresentativeNodeID:   rep,
		SupportingNodeIDs:      supporting,
		ConflictingNodeIDs:     conflicting,
		ConditionalNodeIDs:     conditional,
		PredictiveNodeIDs:      predictive,
		CoreSupportingNodeIDs:  coreSupporting,
		CoreConditionalNodeIDs: coreConditional,
		CoreConclusionNodeIDs:  coreConclusions,
		CorePredictiveNodeIDs:  corePredictive,
		ExpandedNodeIDs:        expanded,
		SynthesizedEdges:       buildSynthesizedEdges(coreSupporting, coreConditional, coreConclusions, corePredictive),
		Active:                 true,
		UpdatedAt:              now,
	}
}

func deriveContradictionProposition(conflicting []string, byID map[string]memory.AcceptedNode) (string, bool) {
	if len(conflicting) < 2 {
		return "", false
	}
	left := canonicalNodeText(strings.TrimSpace(byID[conflicting[0]].NodeText))
	right := canonicalNodeText(strings.TrimSpace(byID[conflicting[1]].NodeText))
	if left == "" || right == "" {
		return "", false
	}
	phrase := longestCommonSubstring([]rune(left), []rune(right))
	phrase = strings.TrimSpace(phrase)
	if len([]rune(phrase)) < 2 {
		phrase = truncateText(left, 20)
	}
	if phrase == "" {
		return "", false
	}
	return "关于「" + phrase + "」的判断", true
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
	if proposition, ok := deriveMacroThemeProposition(component, byID); ok {
		return proposition
	}
	if proposition, ok := deriveNeutralProposition(component, byID); ok {
		return proposition
	}
	repID := chooseRepresentativeNode(component, byID)
	text := normalizedClusterText(byID[repID].NodeText)
	if text == "" {
		return "未命名认知簇"
	}
	return truncateText(text, 80)
}

func deriveMacroThemeProposition(component []string, byID map[string]memory.AcceptedNode) (string, bool) {
	texts := make([]string, 0, len(component))
	for _, id := range component {
		if text := strings.TrimSpace(byID[id].NodeText); text != "" {
			texts = append(texts, canonicalNodeText(text))
		}
	}
	if len(texts) < 2 {
		return "", false
	}
	hasAny := func(needles ...string) bool {
		for _, text := range texts {
			for _, needle := range needles {
				if strings.Contains(text, needle) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case hasAny("银行去监管", "去监管", "金融体系安全", "银行体系更安全"):
		return "关于「银行监管与金融系统安全」的判断", true
	case hasAny("石油美元", "油价", "霍尔木兹", "伊朗", "中东") && hasAny("流动性", "挤兑", "私募信贷", "美债", "美股", "供应链", "大宗商品"):
		return "关于「石油美元、油价与流动性风险」的判断", true
	case hasAny("债务", "金融资产", "货币", "央行", "资产价格", "利率") && hasAny("购买力", "贬值", "通胀", "回报", "脆弱性"):
		return "关于「债务周期与金融资产实际回报」的判断", true
	case hasAny("黑天鹅", "挤兑", "系统性", "危机") && hasAny("华尔街", "金融市场", "联储", "qe", "量化宽松"):
		return "关于「系统性金融风险与政策应对」的判断", true
	default:
		return "", false
	}
}

func deriveNeutralProposition(component []string, byID map[string]memory.AcceptedNode) (string, bool) {
	if len(component) < 2 {
		return "", false
	}
	var canonicals []string
	for _, id := range component {
		text := normalizedClusterText(byID[id].NodeText)
		if text == "" {
			continue
		}
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

func buildClusterSummary(canonical string, supporting, conflicting, conditional, predictive []string, byID map[string]memory.AcceptedNode) string {
	parts := make([]string, 0, 4)
	if trimmed := strings.TrimSpace(canonical); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if text := summarizeRoleTexts(supporting, byID); text != "" {
		parts = append(parts, "支持信息包括"+text)
	}
	if text := summarizeRoleTexts(conditional, byID); text != "" {
		parts = append(parts, "条件包括"+text)
	}
	if text := summarizeRoleTexts(predictive, byID); text != "" {
		parts = append(parts, "相关预测包括"+text)
	}
	if text := summarizeRoleTexts(conflicting, byID); text != "" {
		parts = append(parts, "但也存在冲突观点："+text)
	}
	if len(parts) == 0 {
		return "未命名认知簇"
	}
	return truncateText(strings.Join(parts, "；"), 180)
}

func selectCoreNodes(ids []string, byID map[string]memory.AcceptedNode, max int) []string {
	if len(ids) <= max {
		out := cloneStringSlice(ids)
		sort.Strings(out)
		return out
	}
	sorted := cloneStringSlice(ids)
	sort.Slice(sorted, func(i, j int) bool {
		left := strings.TrimSpace(byID[sorted[i]].NodeText)
		right := strings.TrimSpace(byID[sorted[j]].NodeText)
		if len([]rune(left)) != len([]rune(right)) {
			return len([]rune(left)) > len([]rune(right))
		}
		return sorted[i] < sorted[j]
	})
	out := cloneStringSlice(sorted[:max])
	sort.Strings(out)
	return out
}

func filterNodesByKind(component []string, byID map[string]memory.AcceptedNode, kind string) []string {
	out := make([]string, 0)
	for _, id := range component {
		if byID[id].NodeKind == kind {
			out = append(out, id)
		}
	}
	return out
}

func buildSynthesizedEdges(coreSupporting, coreConditional, coreConclusions, corePredictive []string) []memory.GlobalClusterEdge {
	edges := make([]memory.GlobalClusterEdge, 0)
	add := func(froms, tos []string, kind string) {
		for _, from := range froms {
			for _, to := range tos {
				if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || from == to {
					continue
				}
				edges = append(edges, memory.GlobalClusterEdge{From: from, To: to, Kind: kind})
			}
		}
	}
	add(coreSupporting, coreConclusions, "supporting->conclusion")
	add(coreConditional, coreConclusions, "conditional->conclusion")
	add(coreConclusions, corePredictive, "conclusion->prediction")
	add(coreConditional, corePredictive, "conditional->prediction")
	return edges
}

func summarizeRoleTexts(ids []string, byID map[string]memory.AcceptedNode) string {
	if len(ids) == 0 {
		return ""
	}
	items := make([]string, 0, 2)
	for _, id := range ids {
		text := normalizedClusterText(byID[id].NodeText)
		if text == "" {
			continue
		}
		items = append(items, truncateText(text, 40))
		if len(items) == 2 {
			break
		}
	}
	return strings.Join(items, "；")
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

func longestCommonSubstring(a, b []rune) string {
	if len(a) == 0 || len(b) == 0 {
		return ""
	}
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	bestLen := 0
	bestEnd := 0
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
				if dp[i][j] > bestLen {
					bestLen = dp[i][j]
					bestEnd = i
				}
			}
		}
	}
	if bestLen == 0 {
		return ""
	}
	return string(a[bestEnd-bestLen : bestEnd])
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

func normalizedClusterText(text string) string {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"若", "如果", "一旦", "假如"} {
		text = strings.TrimPrefix(text, prefix)
	}
	return strings.TrimSpace(text)
}

func globalizeAcceptedNodes(nodes []memory.AcceptedNode) []memory.AcceptedNode {
	globalNodes := make([]memory.AcceptedNode, 0, len(nodes))
	for _, node := range nodes {
		node.NodeID = globalMemoryNodeRef(node)
		globalNodes = append(globalNodes, node)
	}
	return globalNodes
}

func splitAcceptedNodesByActivity(nodes []memory.AcceptedNode, now time.Time) ([]memory.AcceptedNode, []memory.AcceptedNode) {
	active := make([]memory.AcceptedNode, 0, len(nodes))
	inactive := make([]memory.AcceptedNode, 0, len(nodes))
	for _, node := range nodes {
		if isAcceptedNodeActiveAt(node, now) {
			active = append(active, node)
		} else {
			inactive = append(inactive, node)
		}
	}
	return active, inactive
}
