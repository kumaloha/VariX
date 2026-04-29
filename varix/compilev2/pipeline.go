package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
)

type graphRole string

const (
	roleDriver       graphRole = "driver"
	roleTransmission graphRole = "transmission"
	roleOrphan       graphRole = "orphan"
)

type graphNode struct {
	ID            string
	Text          string
	SourceQuote   string
	Role          graphRole
	DiscourseRole string
	Ontology      string
	IsTarget      bool
}

type graphEdge struct {
	From        string
	To          string
	Kind        string
	SourceQuote string
	Reason      string
}

type auxEdge struct {
	From        string
	To          string
	Kind        string
	SourceQuote string
	Reason      string
}

type offGraphItem struct {
	ID          string
	Text        string
	Role        string
	AttachesTo  string
	SourceQuote string
}

type graphState struct {
	Nodes       []graphNode
	Edges       []graphEdge
	AuxEdges    []auxEdge
	OffGraph    []offGraphItem
	BranchHeads []string
	Spines      []PreviewSpine
	ArticleForm string
	Rounds      int
}

type relationKind string

const (
	relationCausal      relationKind = "causal"
	relationSupports    relationKind = "supports"
	relationSupplements relationKind = "supplements"
	relationExplains    relationKind = "explains"
	relationNone        relationKind = "none"
)

func countRole(state graphState, role graphRole) int {
	count := 0
	for _, n := range state.Nodes {
		if n.Role == role {
			count++
		}
	}
	return count
}

func countTargets(state graphState) int {
	count := 0
	for _, n := range state.Nodes {
		if n.IsTarget {
			count++
		}
	}
	return count
}

func stage1Extract(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle) (graphState, error) {
	systemPrompt, err := renderStage1SystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage1UserPrompt(bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var payload map[string]any
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "extract", &payload); err != nil {
		return graphState{}, err
	}
	state := graphState{}
	state.ArticleForm = normalizeArticleForm(asString(payload["article_form"]))
	state.Nodes = decodeStage1Nodes(payload["nodes"])
	state.Edges = nil
	state.OffGraph = decodeStage1OffGraph(payload["off_graph"])
	state = fillMissingStage1IDs(state)
	state.ArticleForm = refineArticleFormFromExtract(bundle, state)
	return state, nil
}

func stage1Refine(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	state = normalizeStage1State(state)
	if len(state.Nodes) == 0 {
		return state, nil
	}
	candidates := refineCandidateNodes(state.Nodes)
	if len(candidates) == 0 {
		return state, nil
	}
	systemPrompt, err := renderRefineSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderRefineUserPrompt(bundle.TextContext(), serializeNodeList(candidates))
	if err != nil {
		return graphState{}, err
	}
	var result refineResult
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "refine", &result); err != nil {
		return graphState{}, err
	}
	return applyRefineReplacements(state, result.Replacements), nil
}

type refineResult struct {
	Replacements []refineReplacement `json:"replacements"`
}

type refineReplacement struct {
	ReplaceID    string                  `json:"replace_id"`
	RelationType string                  `json:"relation_type"`
	Nodes        []refineReplacementNode `json:"nodes"`
	Edges        []refineReplacementEdge `json:"edges"`
	Reason       string                  `json:"reason"`
}

type refineReplacementNode struct {
	Text          string `json:"text"`
	SourceQuote   string `json:"source_quote"`
	DiscourseRole string `json:"role"`
}

type refineReplacementEdge struct {
	FromIndex   int    `json:"from_index"`
	ToIndex     int    `json:"to_index"`
	Kind        string `json:"kind"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}

type aggregateResult struct {
	Aggregates []aggregatePatch `json:"aggregates"`
}

type aggregatePatch struct {
	Text        string   `json:"text"`
	MemberIDs   []string `json:"member_ids"`
	SourceQuote string   `json:"source_quote"`
	Reason      string   `json:"reason"`
}

func stage1Aggregate(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	state = normalizeStage1State(state)
	if len(state.Nodes) == 0 {
		return state, nil
	}
	candidates := serializeAggregateCandidateGroups(state.Nodes)
	if strings.TrimSpace(candidates) == "" {
		return state, nil
	}
	systemPrompt, err := renderAggregateSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderAggregateUserPrompt(bundle.TextContext(), candidates)
	if err != nil {
		return graphState{}, err
	}
	var result aggregateResult
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "aggregate", &result); err != nil {
		return graphState{}, err
	}
	return applyAggregatePatches(state, result.Aggregates), nil
}

func serializeAggregateCandidateGroups(nodes []graphNode) string {
	type group struct {
		quote     string
		nodes     []graphNode
		suggested string
	}
	groupsByQuote := map[string][]graphNode{}
	order := make([]string, 0)
	for _, node := range nodes {
		quote := strings.TrimSpace(node.SourceQuote)
		if quote == "" {
			continue
		}
		if _, ok := groupsByQuote[quote]; !ok {
			order = append(order, quote)
		}
		groupsByQuote[quote] = append(groupsByQuote[quote], node)
	}
	groups := make([]group, 0, len(order))
	for _, quote := range order {
		items := groupsByQuote[quote]
		if len(items) < 3 {
			continue
		}
		if !looksLikeAggregateCandidateQuote(quote) {
			continue
		}
		groups = append(groups, group{quote: quote, nodes: items, suggested: suggestAggregateLabel(items, quote)})
	}
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	for i, group := range groups {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Group %d quote: %s\n", i+1, group.quote)
		if strings.TrimSpace(group.suggested) != "" {
			fmt.Fprintf(&b, "Suggested aggregate label: %s\n", group.suggested)
		}
		for _, node := range group.nodes {
			fmt.Fprintf(&b, "- %s: %s\n", node.ID, node.Text)
		}
	}
	return b.String()
}

func looksLikeAggregateCandidateQuote(quote string) bool {
	lower := strings.ToLower(strings.TrimSpace(quote))
	return containsAnyText(lower, []string{
		"、", "，", ",", "和", "以及", "等", "一连串", "全都", "统统", "所有", "各类", "多个", "both", "and", "as well as",
	})
}

func suggestAggregateLabel(nodes []graphNode, quote string) string {
	texts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		texts = append(texts, strings.TrimSpace(node.Text))
	}
	switch {
	case countTextsContaining(texts, "价格被压低") >= 2 && aggregateQuoteContainsAny(quote, []string{"资产", "股票", "债券", "房产", "私募"}):
		return "资产价格被压低"
	case countTextsContaining(texts, "价格下跌") >= 2 && aggregateQuoteContainsAny(quote, []string{"资产", "股票", "债券", "房产", "私募"}):
		return "资产价格下跌"
	case countTextsContaining(texts, "价格承压") >= 2 && aggregateQuoteContainsAny(quote, []string{"资产", "股票", "债券", "房产", "私募"}):
		return "资产价格承压"
	case countTextsContaining(texts, "成本上升") >= 2:
		return "下游成本上升"
	case countTextsContaining(texts, "成本维持高位") >= 2 || countTextsContaining(texts, "成本难降") >= 2:
		return "融资成本维持高位"
	case countTextsContaining(texts, "受影响") >= 2:
		return "政府支出项目受影响"
	default:
		return ""
	}
}

func countTextsContaining(texts []string, marker string) int {
	count := 0
	for _, text := range texts {
		if strings.Contains(strings.ToLower(text), strings.ToLower(marker)) {
			count++
		}
	}
	return count
}

func aggregateQuoteContainsAny(quote string, markers []string) bool {
	return containsAnyText(strings.ToLower(quote), markers)
}

func applyAggregatePatches(state graphState, aggregates []aggregatePatch) graphState {
	if len(aggregates) == 0 {
		return state
	}
	valid := map[string]graphNode{}
	for _, node := range state.Nodes {
		valid[node.ID] = node
	}
	existingText := map[string]struct{}{}
	for _, node := range state.Nodes {
		existingText[normalizeText(node.Text)] = struct{}{}
	}
	nextIndex := 1
	for _, aggregate := range aggregates {
		text := strings.TrimSpace(aggregate.Text)
		if text == "" || containsAnyText(strings.ToLower(text), aggregateForbiddenMarkers()) {
			continue
		}
		memberIDs := dedupeStrings(aggregate.MemberIDs)
		if len(memberIDs) < 2 {
			continue
		}
		validMembers := make([]string, 0, len(memberIDs))
		for _, id := range memberIDs {
			if _, ok := valid[id]; ok {
				validMembers = append(validMembers, id)
			}
		}
		if len(validMembers) < 2 {
			continue
		}
		key := normalizeText(text)
		if _, exists := existingText[key]; exists {
			continue
		}
		id := fmt.Sprintf("agg_%d", nextIndex)
		nextIndex++
		state.Nodes = append(state.Nodes, graphNode{
			ID:            id,
			Text:          text,
			SourceQuote:   strings.TrimSpace(aggregate.SourceQuote),
			DiscourseRole: aggregateDiscourseRole(validMembers, valid),
		})
		existingText[key] = struct{}{}
		for _, memberID := range validMembers {
			state.AuxEdges = append(state.AuxEdges, auxEdge{
				From:        memberID,
				To:          id,
				Kind:        "supplementary",
				SourceQuote: strings.TrimSpace(aggregate.SourceQuote),
				Reason:      strings.TrimSpace(aggregate.Reason),
			})
		}
	}
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	state.BranchHeads = nil
	return state
}

func aggregateForbiddenMarkers() []string {
	return []string{"导致", "引发", "造成", "使", "影响", "推高", "推动", "压低", "拖累", "传导", "drive", "drives", "cause", "causes", "lead to", "leads to"}
}

func aggregateDiscourseRole(memberIDs []string, nodeIndex map[string]graphNode) string {
	best := ""
	bestScore := -1
	for _, id := range memberIDs {
		role := normalizeDiscourseRole(nodeIndex[id].DiscourseRole)
		score := discourseRolePriority(role)
		if score > bestScore {
			best = role
			bestScore = score
		}
	}
	if best == "" {
		return "mechanism"
	}
	return best
}

func refineCandidateNodes(nodes []graphNode) []graphNode {
	out := make([]graphNode, 0, len(nodes))
	for _, node := range nodes {
		if needsRefineCheck(node.Text) {
			out = append(out, node)
		}
	}
	return out
}

func needsRefineCheck(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	return containsAnyText(lower, refineCausalHints()) || containsAnyText(lower, refineParallelHints())
}

func containsAnyText(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func refineCausalHints() []string {
	return []string{
		"导致", "引发", "触发", "造成", "使得", "令", "让", "影响", "推动", "驱动", "带动", "拉动", "支撑", "压制", "拖累", "打压", "抑制", "压低", "削弱", "强化", "放大", "缓解", "加剧", "吸引", "抽走", "转移", "虹吸", "挤压", "重配", "推高", "抬升", "压缩", "扩大", "收窄", "扰动", "冲击", "外溢", "传导", "重定价", "被堵", "堵在",
		"cause", "lead to", "trigger", "result in", "create", "drive", "push", "pull", "support", "sustain", "weaken", "strengthen", "amplify", "reduce", "ease", "worsen", "pressure", "drag", "weigh on", "lift", "suppress", "attract", "drain", "redirect", "reallocate", "widen", "narrow", "compress", "expand", "raise", "lower", "spill over", "transmit", "propagate", "reprice", "reset", "prompt", "induce", "spur",
	}
}

func refineParallelHints() []string {
	return []string{"和", "及", "以及", "与", "并且", "同时", "还有", "且", "其中", "如果", "若", "则", "就", "统统", "都", "、", "，", ",", " and ", " as well as ", " both ", " while ", " along with ", " together with "}
}

func applyRefineReplacements(state graphState, replacements []refineReplacement) graphState {
	patches := map[string][]graphNode{}
	redirect := map[string]string{}
	for _, replacement := range replacements {
		replaceID := strings.TrimSpace(replacement.ReplaceID)
		if replaceID == "" || len(replacement.Nodes) == 0 {
			continue
		}
		nodes := make([]graphNode, 0, len(replacement.Nodes))
		for idx, item := range replacement.Nodes {
			text := strings.TrimSpace(item.Text)
			if text == "" {
				continue
			}
			sourceQuote := strings.TrimSpace(item.SourceQuote)
			nodes = append(nodes, graphNode{
				ID:            fmt.Sprintf("%s_%d", replaceID, idx+1),
				Text:          text,
				SourceQuote:   sourceQuote,
				DiscourseRole: normalizeDiscourseRole(item.DiscourseRole),
			})
		}
		if len(nodes) == 0 {
			continue
		}
		patches[replaceID] = nodes
		redirect[replaceID] = nodes[0].ID
	}
	if len(patches) == 0 {
		return state
	}
	out := make([]graphNode, 0, len(state.Nodes)+len(patches))
	for _, node := range state.Nodes {
		nodes, ok := patches[node.ID]
		if !ok {
			out = append(out, node)
			continue
		}
		for i := range nodes {
			if strings.TrimSpace(nodes[i].SourceQuote) == "" {
				nodes[i].SourceQuote = node.SourceQuote
			}
			if strings.TrimSpace(nodes[i].DiscourseRole) == "" {
				nodes[i].DiscourseRole = node.DiscourseRole
			}
			out = append(out, nodes[i])
		}
	}
	state.Nodes = out
	for i := range state.OffGraph {
		if next := redirect[state.OffGraph[i].AttachesTo]; next != "" {
			state.OffGraph[i].AttachesTo = next
		}
	}
	state.Edges = nil
	state.AuxEdges = nil
	state.BranchHeads = nil
	return fillMissingStage1IDs(state)
}

func normalizeStage1State(state graphState) graphState {
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		n.ID = strings.TrimSpace(n.ID)
		n.Text = strings.TrimSpace(n.Text)
		n.SourceQuote = strings.TrimSpace(n.SourceQuote)
		n.DiscourseRole = normalizeDiscourseRole(n.DiscourseRole)
		if n.Text == "" {
			continue
		}
		nodes = append(nodes, n)
	}
	state.Nodes = nodes
	state.ArticleForm = normalizeArticleForm(state.ArticleForm)

	validIDs := map[string]struct{}{}
	for _, n := range state.Nodes {
		validIDs[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		e.From = strings.TrimSpace(e.From)
		e.To = strings.TrimSpace(e.To)
		if e.From == "" || e.To == "" || e.From == e.To {
			continue
		}
		if _, ok := validIDs[e.From]; !ok {
			continue
		}
		if _, ok := validIDs[e.To]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = edges

	off := make([]offGraphItem, 0, len(state.OffGraph))
	for _, o := range state.OffGraph {
		o.Text = strings.TrimSpace(o.Text)
		if o.Text == "" {
			continue
		}
		o.Role = strings.TrimSpace(o.Role)
		if o.Role == "" {
			o.Role = "supplementary"
		}
		off = append(off, o)
	}
	state.OffGraph = off
	return state
}

func decodeStage1Nodes(raw any) []graphNode {
	items, _ := raw.([]any)
	out := make([]graphNode, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, graphNode{Text: strings.TrimSpace(v)})
		case map[string]any:
			out = append(out, graphNode{
				ID:            strings.TrimSpace(asString(v["id"])),
				Text:          strings.TrimSpace(compile.FirstNonEmpty(asString(v["text"]), asString(v["content"]))),
				SourceQuote:   strings.TrimSpace(asString(v["source_quote"])),
				DiscourseRole: normalizeDiscourseRole(asString(v["role"])),
			})
		}
	}
	return out
}

func normalizeArticleForm(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "single_thesis", "main_narrative_plus_investment_implication", "evidence_backed_forecast", "risk_list", "macro_framework", "market_update", "institutional_satire", "satirical_financial_commentary":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func refineArticleFormFromExtract(bundle compile.Bundle, state graphState) string {
	form := normalizeArticleForm(state.ArticleForm)
	if form != "" && form != "main_narrative_plus_investment_implication" {
		return form
	}
	if satireArticleScore(bundle.TextContext(), state.Nodes) >= 5 {
		return "satirical_financial_commentary"
	}
	if !isLongFormMacroSource(bundle) {
		return form
	}
	if evidenceBackedForecastScore(bundle.TextContext(), state.Nodes) >= 4 {
		return "evidence_backed_forecast"
	}
	if longFormMacroFrameworkScore(bundle.TextContext(), state.Nodes) < 4 {
		return form
	}
	return "macro_framework"
}

func satireArticleScore(article string, nodes []graphNode) int {
	textParts := []string{article}
	analogyRoles := 0
	satireTargetRoles := 0
	for _, node := range nodes {
		textParts = append(textParts, node.Text, node.SourceQuote)
		switch normalizeDiscourseRole(node.DiscourseRole) {
		case "analogy":
			analogyRoles++
		case "satire_target", "implied_thesis":
			satireTargetRoles++
		}
	}
	score := 0
	if analogyRoles > 0 {
		score += 2
	}
	if satireTargetRoles > 0 {
		score += 3
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	for _, family := range [][]string{
		{"讽刺", "satire", "satirical", "irony"},
		{"寓言", "类比", "故事", "analogy", "allegory"},
		{"村长", "新富", "幸运游戏", "抽奖", "幸运观众"},
		{"叙事", "包装成公平", "包装", "忽悠", "牌照"},
	} {
		if containsAnyText(text, family) {
			score++
		}
	}
	return score
}

func evidenceBackedForecastScore(article string, nodes []graphNode) int {
	textParts := []string{article}
	for _, node := range nodes {
		textParts = append(textParts, node.Text, node.SourceQuote)
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	score := 0
	for _, family := range [][]string{
		{"推断", "推导", "可能", "如果", "would", "could", "likely", "probability", "forecast"},
		{"调研", "研究", "证据", "历史", "precedent", "evidence", "research"},
		{"沃什", "warsh"},
		{"美联储", "fed", "federal reserve"},
		{"金融抑制", "金融压抑", "financial repression"},
	} {
		if containsAnyText(text, family) {
			score++
		}
	}
	return score
}

func isLongFormMacroSource(bundle compile.Bundle) bool {
	switch strings.ToLower(strings.TrimSpace(bundle.Source)) {
	case "youtube":
		return true
	default:
		return false
	}
}

func longFormMacroFrameworkScore(article string, nodes []graphNode) int {
	textParts := []string{article}
	for _, node := range nodes {
		textParts = append(textParts, node.Text, node.SourceQuote)
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	score := 0
	for _, family := range longFormMacroFrameworkFamilies() {
		if containsAnyText(text, family) {
			score++
		}
	}
	return score
}

func longFormMacroFrameworkFamilies() [][]string {
	return [][]string{
		{"法币", "fiat"},
		{"信用", "credit"},
		{"债务", "debt"},
		{"人口老龄化", "老龄化", "demographic", "aging"},
		{"税基", "tax base"},
		{"主权债", "主权债务", "sovereign debt"},
		{"金融压抑", "financial repression"},
		{"美元信用", "美元单核", "dollar hegemony"},
		{"outside money", "外部货币", "实物商品"},
	}
}

func normalizeDiscourseRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "thesis", "mechanism", "evidence", "example", "implication", "caveat", "market_move", "analogy", "satire_target", "implied_thesis":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func decodeStage1Edges(raw any) []graphEdge {
	items, _ := raw.([]any)
	out := make([]graphEdge, 0, len(items))
	for _, item := range items {
		v, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, graphEdge{
			From: strings.TrimSpace(compile.FirstNonEmpty(asString(v["from"]), asString(v["source"]))),
			To:   strings.TrimSpace(compile.FirstNonEmpty(asString(v["to"]), asString(v["target"]))),
		})
	}
	return out
}

func decodeStage1OffGraph(raw any) []offGraphItem {
	items, _ := raw.([]any)
	out := make([]offGraphItem, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, offGraphItem{Text: strings.TrimSpace(v), Role: "supplementary"})
		case map[string]any:
			out = append(out, offGraphItem{
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(asString(v["text"])),
				Role:        strings.TrimSpace(asString(v["role"])),
				AttachesTo:  strings.TrimSpace(asString(v["attaches_to"])),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
}

func fillMissingStage1IDs(state graphState) graphState {
	for i := range state.Nodes {
		fillMissingStage1Identity(&state.Nodes[i].ID, fmt.Sprintf("n%d", i+1))
		fillMissingStage1Text(&state.Nodes[i].SourceQuote, state.Nodes[i].Text)
	}
	for i := range state.OffGraph {
		fillMissingStage1Identity(&state.OffGraph[i].ID, fmt.Sprintf("o%d", i+1))
		if strings.TrimSpace(state.OffGraph[i].Role) == "" {
			state.OffGraph[i].Role = "supplementary"
		}
		fillMissingStage1Text(&state.OffGraph[i].SourceQuote, state.OffGraph[i].Text)
	}
	return state
}

func fillMissingStage1Identity(field *string, fallback string) {
	if field == nil || strings.TrimSpace(*field) != "" {
		return
	}
	*field = fallback
}

func fillMissingStage1Text(field *string, fallback string) {
	if field == nil || strings.TrimSpace(*field) != "" {
		return
	}
	*field = fallback
}

func stage3Classify(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if projected, ok := projectRolesFromSpines(state); ok {
		return pruneDanglingEdges(projected), nil
	}
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	for _, n := range state.Nodes {
		inDegree[n.ID] = 0
		outDegree[n.ID] = 0
	}
	for _, e := range state.Edges {
		outDegree[e.From]++
		inDegree[e.To]++
	}
	for i := range state.Nodes {
		n := &state.Nodes[i]
		n.IsTarget = false
		n.Ontology = ""
		switch {
		case inDegree[n.ID] == 0 && outDegree[n.ID] > 0:
			n.Role = roleDriver
		case inDegree[n.ID] > 0:
			n.Role = roleTransmission
		default:
			n.Role = roleOrphan
		}
		n.IsTarget = n.Role == roleTransmission && isEligibleTargetHead(n.Text)
		n.Ontology = inferTargetKind(n.Text, n.IsTarget)
	}
	state = pruneDanglingEdges(state)
	return state, nil
}

func projectRolesFromSpines(state graphState) (graphState, bool) {
	if len(state.Spines) == 0 {
		return state, false
	}
	valid := map[string]struct{}{}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		valid[node.ID] = struct{}{}
		nodeIndex[node.ID] = node
	}
	driverIDs := map[string]struct{}{}
	targetIDs := map[string]struct{}{}
	spineNodeIDs := map[string]struct{}{}
	for _, spine := range state.Spines {
		nodes := validSpineNodeIDs(spine, valid)
		if len(nodes) == 0 {
			continue
		}
		markSpineProjectionNodes(spine, nodes, valid, nodeIndex, spineNodeIDs)
		sources, terminals := spineSourceAndTerminalIDs(spine, nodes, valid, nodeIndex)
		for _, id := range sources {
			driverIDs[id] = struct{}{}
		}
		for _, id := range terminals {
			targetIDs[id] = struct{}{}
		}
	}
	if len(spineNodeIDs) == 0 {
		return state, false
	}
	for i := range state.Nodes {
		n := &state.Nodes[i]
		n.IsTarget = false
		n.Ontology = ""
		switch {
		case hasID(driverIDs, n.ID):
			n.Role = roleDriver
		case hasID(spineNodeIDs, n.ID):
			n.Role = roleTransmission
		default:
			n.Role = roleOrphan
		}
		n.IsTarget = hasID(targetIDs, n.ID)
		n.Ontology = inferTargetKind(n.Text, n.IsTarget)
	}
	return state, true
}

func markSpineProjectionNodes(spine PreviewSpine, nodeIDs []string, valid map[string]struct{}, nodes map[string]graphNode, out map[string]struct{}) {
	if normalizePreviewSpinePolicy(spine.Policy) != "satirical_analogy" {
		for _, id := range nodeIDs {
			out[id] = struct{}{}
		}
		return
	}
	before := len(out)
	for _, edge := range spineProjectionEdges(spine, nodes) {
		if _, ok := valid[edge.From]; ok {
			out[edge.From] = struct{}{}
		}
		if _, ok := valid[edge.To]; ok {
			out[edge.To] = struct{}{}
		}
	}
	if len(out) == before {
		for _, id := range nodeIDs {
			out[id] = struct{}{}
		}
	}
}

func validSpineNodeIDs(spine PreviewSpine, valid map[string]struct{}) []string {
	out := make([]string, 0, len(spine.NodeIDs))
	seen := map[string]struct{}{}
	for _, id := range spine.NodeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := valid[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func spineSourceAndTerminalIDs(spine PreviewSpine, nodeIDs []string, valid map[string]struct{}, nodes map[string]graphNode) ([]string, []string) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	nodeSet := map[string]struct{}{}
	for _, id := range nodeIDs {
		nodeSet[id] = struct{}{}
	}
	for _, edge := range spineProjectionEdges(spine, nodes) {
		if _, ok := valid[edge.From]; !ok {
			continue
		}
		if _, ok := valid[edge.To]; !ok {
			continue
		}
		if _, ok := nodeSet[edge.From]; !ok {
			continue
		}
		if _, ok := nodeSet[edge.To]; !ok {
			continue
		}
		if edge.From == edge.To {
			continue
		}
		outDegree[edge.From]++
		inDegree[edge.To]++
	}
	if len(inDegree) == 0 && len(outDegree) == 0 {
		return []string{nodeIDs[0]}, []string{nodeIDs[len(nodeIDs)-1]}
	}
	sources := make([]string, 0)
	terminals := make([]string, 0)
	for _, id := range nodeIDs {
		if outDegree[id] > 0 && inDegree[id] == 0 {
			sources = append(sources, id)
		}
		if inDegree[id] > 0 && outDegree[id] == 0 {
			terminals = append(terminals, id)
		}
	}
	if len(sources) == 0 {
		sources = append(sources, nodeIDs[0])
	}
	if len(terminals) == 0 {
		terminals = append(terminals, nodeIDs[len(nodeIDs)-1])
	}
	return sources, terminals
}

func spineProjectionEdges(spine PreviewSpine, nodes map[string]graphNode) []PreviewEdge {
	if normalizePreviewSpinePolicy(spine.Policy) != "satirical_analogy" {
		return spine.Edges
	}
	out := make([]PreviewEdge, 0, len(spine.Edges))
	for _, edge := range spine.Edges {
		fromRole := normalizeDiscourseRole(nodes[edge.From].DiscourseRole)
		if fromRole == "analogy" || fromRole == "example" {
			continue
		}
		out = append(out, edge)
	}
	if len(out) == 0 {
		return spine.Edges
	}
	return out
}

func isIllustrationKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "illustration")
}

func hasID(values map[string]struct{}, id string) bool {
	_, ok := values[id]
	return ok
}

type mainlineSpinePatch struct {
	ID          string   `json:"id"`
	Level       string   `json:"level"`
	Priority    int      `json:"priority"`
	Policy      string   `json:"policy"`
	Thesis      string   `json:"thesis"`
	NodeIDs     []string `json:"node_ids"`
	EdgeIndexes []int    `json:"edge_indexes"`
	Scope       string   `json:"scope"`
	Why         string   `json:"why"`
}

type spinePolicy struct {
	ArticleForm                    string
	PrimaryMode                    string
	MinSpines                      int
	MaxSpines                      int
	MaxLocal                       int
	PreserveInvestmentImplications bool
	PreserveRiskFamilies           bool
	MergeSameFamilyBranches        bool
	AllowSingleNodeFamilySpines    bool
}

func policyForArticleForm(articleForm string) spinePolicy {
	form := normalizeArticleForm(articleForm)
	switch form {
	case "risk_list":
		return spinePolicy{
			ArticleForm:                 form,
			PrimaryMode:                 "none",
			MinSpines:                   3,
			MaxSpines:                   7,
			MaxLocal:                    1,
			PreserveRiskFamilies:        true,
			MergeSameFamilyBranches:     true,
			AllowSingleNodeFamilySpines: true,
		}
	case "main_narrative_plus_investment_implication":
		return spinePolicy{
			ArticleForm:                    form,
			PrimaryMode:                    "required",
			MinSpines:                      2,
			MaxSpines:                      5,
			MaxLocal:                       1,
			PreserveInvestmentImplications: true,
			MergeSameFamilyBranches:        true,
		}
	case "evidence_backed_forecast":
		return spinePolicy{
			ArticleForm:                    form,
			PrimaryMode:                    "required",
			MinSpines:                      2,
			MaxSpines:                      5,
			MaxLocal:                       1,
			PreserveInvestmentImplications: true,
			MergeSameFamilyBranches:        true,
		}
	case "institutional_satire", "satirical_financial_commentary":
		return spinePolicy{
			ArticleForm:             form,
			PrimaryMode:             "required",
			MinSpines:               2,
			MaxSpines:               5,
			MaxLocal:                1,
			MergeSameFamilyBranches: true,
		}
	case "macro_framework":
		return spinePolicy{
			ArticleForm:             form,
			PrimaryMode:             "required",
			MinSpines:               2,
			MaxSpines:               4,
			MaxLocal:                1,
			MergeSameFamilyBranches: true,
		}
	case "market_update":
		return spinePolicy{
			ArticleForm:                    form,
			PrimaryMode:                    "required",
			MinSpines:                      2,
			MaxSpines:                      5,
			MaxLocal:                       1,
			PreserveInvestmentImplications: true,
			MergeSameFamilyBranches:        true,
		}
	default:
		return spinePolicy{
			ArticleForm:             form,
			PrimaryMode:             "required",
			MinSpines:               1,
			MaxSpines:               3,
			MaxLocal:                1,
			MergeSameFamilyBranches: true,
		}
	}
}

func renderSpinePolicyPrompt(articleForm string) string {
	policy := policyForArticleForm(articleForm)
	switch policy.ArticleForm {
	case "risk_list":
		return "risk_list: preserve each major risk family as a branch spine; do not force or promote a primary spine; priority 1 is only the lead display branch; allow single-node risk-family spines when no grounded downstream endpoint exists; merge within a risk family, not across unrelated families."
	case "main_narrative_plus_investment_implication":
		return "main_narrative_plus_investment_implication: keep one primary narrative spine plus branch spines for derived investment implications; do not collapse investment advice into the primary spine when it is a distinct author conclusion; merge same-function local market implications."
	case "evidence_backed_forecast":
		return "evidence_backed_forecast: the article uses research clues, historical precedent, policy signals, legal feasibility, or quantitative indicators to infer a future regime/outcome. Keep proof branches as inference relations into the forecast thesis, then keep causal branches from that forecast thesis into market or investment implications. The primary spine may be research/policy evidence -> inferred thesis -> implications; mark proof edges as kind=inference."
	case "institutional_satire", "satirical_financial_commentary":
		return "institutional_satire/satirical_financial_commentary: preserve mixed spine policies. A satire spine should be policy=satirical_analogy and follow satire vehicle/allegory -> mapped institutional mechanism -> implied critique or real-world implication. Use kind=illustration from the allegory/example into the real mechanism; do not make the allegory character the economic driver. Keep ordinary causal, forecast, concept, and investment branch spines when the article also contains them."
	case "macro_framework":
		return "macro_framework: keep one framework primary plus summary-level mechanism branches; do not turn section order or historical examples into causal order; preserve mechanism families and demote mere examples."
	case "market_update":
		return "market_update: keep one lead market spine plus parallel asset/factor branches; do not force stocks, bonds, consumer confidence, and policy uncertainty into one chain unless the article directly links them."
	default:
		return "single_thesis/default: keep the shortest sufficient primary causal spine, with only major derived branch spines."
	}
}

func stage2Supplement(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2SupplementSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2SupplementUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		SupplementLinks []struct {
			A           string `json:"a"`
			B           string `json:"b"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"supplement_links"`
		SameThingLinks []struct {
			A           string `json:"a"`
			B           string `json:"b"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"same_thing_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "supplement", &result); err != nil {
		return graphState{}, err
	}
	valid := map[string]graphNode{}
	for _, n := range state.Nodes {
		valid[n.ID] = n
	}
	links := append(result.SupplementLinks, result.SameThingLinks...)
	auxEdges := make([]auxEdge, 0, len(links)*2)
	for _, e := range links {
		if _, ok := valid[e.A]; !ok {
			continue
		}
		if _, ok := valid[e.B]; !ok {
			continue
		}
		if e.A == e.B {
			continue
		}
		auxEdges = append(auxEdges, auxEdge{
			From:        e.A,
			To:          e.B,
			Kind:        "supplementary",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
		auxEdges = append(auxEdges, auxEdge{
			From:        e.B,
			To:          e.A,
			Kind:        "supplementary",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	state.AuxEdges = append(state.AuxEdges, auxEdges...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	state.BranchHeads = nil
	return state, nil
}

type supportEdgeResult struct {
	SupportEdges []supportEdgePatch `json:"support_edges"`
}

type supportEdgePatch struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}

func stage2Support(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2SupportSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2SupportUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result supportEdgeResult
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "support", &result); err != nil {
		return graphState{}, err
	}
	state.AuxEdges = append(state.AuxEdges, buildAuxEdgesFromSupportEdges(state.Nodes, result.SupportEdges)...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	state.BranchHeads = nil
	return state, nil
}

func stage2Evidence(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2EvidenceSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2EvidenceUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		SupportLinks []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"support_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "evidence", &result); err != nil {
		return graphState{}, err
	}
	state.AuxEdges = append(state.AuxEdges, buildAuxEdgesFromSupport(state.Nodes, result.SupportLinks)...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	return state, nil
}

func stage2Explanation(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2ExplanationSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2ExplanationUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		ExplanationLinks []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"explanation_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "explanation", &result); err != nil {
		return graphState{}, err
	}
	state.AuxEdges = append(state.AuxEdges, buildAuxEdgesFromExplanation(state.Nodes, result.ExplanationLinks)...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	return state, nil
}

func collapseClusters(state graphState) graphState {
	if len(state.Nodes) == 0 {
		return state
	}
	if len(state.AuxEdges) == 0 {
		state = demoteSupportingRoleNodes(state)
		if len(state.BranchHeads) == 0 {
			state.BranchHeads = nil
			for _, node := range state.Nodes {
				state.BranchHeads = append(state.BranchHeads, node.ID)
			}
			state.BranchHeads = dedupeStrings(state.BranchHeads)
		}
		return pruneDanglingEdges(state)
	}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		nodeIndex[node.ID] = node
	}
	undirected := map[string][]string{}
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	nodeRole := map[string]string{}
	for _, edge := range state.AuxEdges {
		if _, ok := nodeIndex[edge.From]; !ok {
			continue
		}
		if _, ok := nodeIndex[edge.To]; !ok {
			continue
		}
		undirected[edge.From] = append(undirected[edge.From], edge.To)
		undirected[edge.To] = append(undirected[edge.To], edge.From)
		outDegree[edge.From]++
		inDegree[edge.To]++
		if role, ok := auxNodeRole(edge, edge.From); ok {
			nodeRole[edge.From] = role
		}
		if role, ok := auxNodeRole(edge, edge.To); ok {
			if _, exists := nodeRole[edge.To]; !exists {
				nodeRole[edge.To] = role
			}
		}
	}
	visited := map[string]struct{}{}
	heads := make([]string, 0)
	keep := map[string]struct{}{}
	offGraph := append([]offGraphItem(nil), state.OffGraph...)
	for _, node := range state.Nodes {
		if _, seen := visited[node.ID]; seen {
			continue
		}
		component := collectAuxComponent(undirected, node.ID, visited)
		headID := chooseClusterHead(component, state.AuxEdges, nodeIndex)
		heads = append(heads, headID)
		keep[headID] = struct{}{}
		for _, memberID := range component {
			if memberID == headID {
				continue
			}
			member := nodeIndex[memberID]
			role := nodeRole[memberID]
			if strings.TrimSpace(role) == "" {
				role = "supplementary"
			}
			offGraph = append(offGraph, offGraphItem{
				ID:          fmt.Sprintf("cluster_%s_%s", role, memberID),
				Text:        member.Text,
				Role:        role,
				AttachesTo:  headID,
				SourceQuote: member.SourceQuote,
			})
		}
	}
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, node := range state.Nodes {
		if _, ok := keep[node.ID]; ok {
			nodes = append(nodes, node)
		}
	}
	state.Nodes = nodes
	state.BranchHeads = dedupeStrings(heads)
	state.OffGraph = offGraph
	state = demoteSupportingRoleNodes(state)
	state = pruneDanglingEdges(state)
	return state
}

func demoteSupportingRoleNodes(state graphState) graphState {
	if len(state.Nodes) == 0 || !hasMainlineDiscourseNode(state.Nodes) {
		return state
	}
	attachTo := firstMainlineDiscourseNodeID(state.Nodes)
	nodes := make([]graphNode, 0, len(state.Nodes))
	branchHeads := make([]string, 0, len(state.BranchHeads))
	demoted := map[string]struct{}{}
	for _, node := range state.Nodes {
		if !isSupportingDiscourseRole(node.DiscourseRole) || preserveSupportingDiscourseRole(state.ArticleForm, node.DiscourseRole) {
			nodes = append(nodes, node)
			continue
		}
		role := normalizeDiscourseRole(node.DiscourseRole)
		if role == "" {
			role = "supplementary"
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:          fmt.Sprintf("discourse_%s_%s", role, node.ID),
			Text:        node.Text,
			Role:        role,
			AttachesTo:  attachTo,
			SourceQuote: node.SourceQuote,
		})
		demoted[node.ID] = struct{}{}
	}
	if len(demoted) == 0 {
		return state
	}
	for _, id := range state.BranchHeads {
		if _, ok := demoted[id]; ok {
			continue
		}
		branchHeads = append(branchHeads, id)
	}
	state.Nodes = nodes
	state.BranchHeads = dedupeStrings(branchHeads)
	return state
}

func preserveSupportingDiscourseRole(articleForm, role string) bool {
	switch normalizeArticleForm(articleForm) {
	case "risk_list":
		return normalizeDiscourseRole(role) == "caveat"
	default:
		return false
	}
}

func hasMainlineDiscourseNode(nodes []graphNode) bool {
	return firstMainlineDiscourseNodeID(nodes) != ""
}

func firstMainlineDiscourseNodeID(nodes []graphNode) string {
	for _, node := range nodes {
		if isMainlineDiscourseRole(node.DiscourseRole) {
			return node.ID
		}
	}
	return ""
}

func isMainlineDiscourseRole(role string) bool {
	switch normalizeDiscourseRole(role) {
	case "thesis", "mechanism", "implication", "market_move", "satire_target", "implied_thesis":
		return true
	default:
		return false
	}
}

func isSupportingDiscourseRole(role string) bool {
	switch normalizeDiscourseRole(role) {
	case "evidence", "example", "caveat":
		return true
	default:
		return false
	}
}

func stage3Mainline(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage3MainlineSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage3MainlineUserPrompt(bundle.TextContext(), state.ArticleForm, serializeRelationNodes(state.Nodes), serializeBranchHeads(state), serializeMainlineCandidateEdges(bundle.TextContext(), state.Nodes))
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		Relations []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			Kind        string `json:"kind"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"relations"`
		LegacyDrivesEdges []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			Kind        string `json:"kind"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"drives_edges"`
		Spines []mainlineSpinePatch `json:"spines"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "mainline", &result); err != nil {
		return graphState{}, err
	}
	valid := map[string]graphNode{}
	for _, n := range state.Nodes {
		valid[n.ID] = n
	}
	relationPatches := result.Relations
	if len(relationPatches) == 0 {
		relationPatches = result.LegacyDrivesEdges
	}
	newEdges := make([]graphEdge, 0, len(relationPatches))
	for _, e := range relationPatches {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		newEdges = append(newEdges, graphEdge{
			From:        e.From,
			To:          e.To,
			Kind:        normalizeMainlineRelationKind(e.Kind),
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	oldEdges := state.Edges
	state.Edges = pruneTransitiveRelations(dedupeEdges(append(append([]graphEdge(nil), oldEdges...), newEdges...)))
	state.Spines = buildSpinesFromLLM(result.Spines, newEdges, state.Edges, valid, state.ArticleForm)
	if len(state.BranchHeads) > 0 {
		keep := collectBranchMainlineNodes(state.Edges, state.BranchHeads)
		demoted := map[string]struct{}{}
		for _, n := range state.Nodes {
			if _, ok := keep[n.ID]; ok {
				continue
			}
			attachTo := chooseBranchAttachment(oldEdges, n.ID, keep, state.BranchHeads)
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID:          fmt.Sprintf("sup_pruned_%s", n.ID),
				Text:        n.Text,
				Role:        "supplementary",
				AttachesTo:  attachTo,
				SourceQuote: n.SourceQuote,
			})
			demoted[n.ID] = struct{}{}
		}
		if len(demoted) > 0 {
			nodes := make([]graphNode, 0, len(state.Nodes))
			for _, n := range state.Nodes {
				if _, ok := demoted[n.ID]; ok {
					continue
				}
				nodes = append(nodes, n)
			}
			state.Nodes = nodes
		}
	}
	state = pruneDanglingEdges(state)
	return state, nil
}

func buildSpinesFromLLM(raw []mainlineSpinePatch, rawEdges []graphEdge, finalEdges []graphEdge, valid map[string]graphNode, articleForm string) []PreviewSpine {
	if len(raw) == 0 {
		return nil
	}
	out := make([]PreviewSpine, 0, len(raw))
	for i, item := range raw {
		nodeIDs := make([]string, 0, len(item.NodeIDs))
		seenNodes := map[string]struct{}{}
		for _, id := range item.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := valid[id]; !ok {
				continue
			}
			if _, ok := seenNodes[id]; ok {
				continue
			}
			seenNodes[id] = struct{}{}
			nodeIDs = append(nodeIDs, id)
		}
		spineEdges := make([]PreviewEdge, 0, len(item.EdgeIndexes))
		seenEdges := map[string]struct{}{}
		for _, edgeIndex := range item.EdgeIndexes {
			if edgeIndex < 0 || edgeIndex >= len(rawEdges) {
				continue
			}
			edge := rawEdges[edgeIndex]
			if _, ok := valid[edge.From]; !ok {
				continue
			}
			if _, ok := valid[edge.To]; !ok {
				continue
			}
			if !hasEdge(finalEdges, edge.From, edge.To) {
				continue
			}
			key := edge.From + "->" + edge.To
			if _, ok := seenEdges[key]; ok {
				continue
			}
			seenEdges[key] = struct{}{}
			spineEdges = append(spineEdges, previewEdgeFromGraphEdge(edge))
		}
		if len(spineEdges) == 0 {
			for _, edge := range finalEdges {
				if _, ok := seenNodes[edge.From]; !ok {
					continue
				}
				if _, ok := seenNodes[edge.To]; !ok {
					continue
				}
				key := edge.From + "->" + edge.To
				if _, ok := seenEdges[key]; ok {
					continue
				}
				seenEdges[key] = struct{}{}
				spineEdges = append(spineEdges, previewEdgeFromGraphEdge(edge))
			}
		}
		if len(nodeIDs) == 0 {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("s%d", len(out)+1)
		}
		level := normalizePreviewSpineLevel(item.Level)
		priority := item.Priority
		if priority <= 0 {
			priority = i + 1
		}
		out = append(out, PreviewSpine{
			ID:       id,
			Level:    level,
			Priority: priority,
			Policy:   normalizePreviewSpinePolicy(item.Policy),
			Thesis:   strings.TrimSpace(item.Thesis),
			NodeIDs:  nodeIDs,
			Edges:    spineEdges,
			Scope:    normalizePreviewSpineScope(item.Scope, level),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	policy := policyForArticleForm(articleForm)
	out = inferMissingSpinePolicies(out, valid, policy)
	out = applySpinePolicy(out, valid, policy)
	out = compactSpines(out, valid)
	out = enforceSpineBudget(out, valid, policy)
	return assignSpineFamilies(out, valid)
}

func inferMissingSpinePolicies(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	for i := range spines {
		current := normalizePreviewSpinePolicy(spines[i].Policy)
		if isSatiricalArticleForm(policy.ArticleForm) && spineHasDiscourseRole(spines[i], valid, "analogy", "satire_target", "implied_thesis") {
			if current == "" || current == "causal_mechanism" {
				spines[i].Policy = "satirical_analogy"
				continue
			}
		}
		if current == "" && spineHasRelationKind(spines[i], "inference") {
			spines[i].Policy = "forecast_inference"
			continue
		}
		spines[i].Policy = current
	}
	return spines
}

func isSatiricalArticleForm(articleForm string) bool {
	switch normalizeArticleForm(articleForm) {
	case "institutional_satire", "satirical_financial_commentary":
		return true
	default:
		return false
	}
}

func spineHasRelationKind(spine PreviewSpine, kind string) bool {
	for _, edge := range spine.Edges {
		if strings.EqualFold(strings.TrimSpace(edge.Kind), strings.TrimSpace(kind)) {
			return true
		}
	}
	return false
}

func applySpinePolicy(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	for i := range spines {
		if policy.PreserveInvestmentImplications && spineHasDiscourseRole(spines[i], valid, "implication", "market_move") && spines[i].Level == "local" {
			spines[i].Level = "branch"
			if spines[i].Scope == "local" {
				spines[i].Scope = "branch"
			}
		}
	}
	switch policy.PrimaryMode {
	case "none":
		for i := range spines {
			if spines[i].Level != "primary" {
				continue
			}
			spines[i].Level = "branch"
			if spines[i].Scope == "article" {
				spines[i].Scope = "branch"
			}
		}
		return renumberSpinePriorities(spines)
	default:
		return enforceSinglePrimarySpine(spines)
	}
}

func spineHasDiscourseRole(spine PreviewSpine, valid map[string]graphNode, roles ...string) bool {
	wanted := map[string]struct{}{}
	for _, role := range roles {
		wanted[normalizeDiscourseRole(role)] = struct{}{}
	}
	for _, id := range spine.NodeIDs {
		node, ok := valid[id]
		if !ok {
			continue
		}
		if _, ok := wanted[normalizeDiscourseRole(node.DiscourseRole)]; ok {
			return true
		}
	}
	return false
}

func renumberSpinePriorities(spines []PreviewSpine) []PreviewSpine {
	for i := range spines {
		spines[i].Priority = i + 1
	}
	return spines
}

func assignSpineFamilies(spines []PreviewSpine, valid map[string]graphNode) []PreviewSpine {
	for i := range spines {
		meta := inferSpineFamily(spines[i], valid)
		spines[i].FamilyKey = meta.Key
		spines[i].FamilyLabel = meta.Label
		spines[i].FamilyScope = meta.Scope
	}
	return spines
}

type spineFamily struct {
	Key   string
	Label string
	Scope string
}

type spineFamilyRule struct {
	Meta        spineFamily
	Markers     []string
	RequiredAny []string
	MinScore    int
}

var spineFamilyRules = []spineFamilyRule{
	{
		Meta:        spineFamily{Key: "bank_regulation_fragmentation", Label: "银行监管碎片化", Scope: "regulation"},
		Markers:     []string{"post-2008", "regulation", "regulations", "fragmented", "productive lending", "basel", "银行监管", "监管", "碎片化", "生产性信贷"},
		RequiredAny: []string{"post-2008", "regulation", "regulations", "basel", "银行监管", "监管"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "war_energy_inflation", Label: "战争能源通胀", Scope: "geopolitics"},
		Markers:     []string{"war", "战争", "oil", "原油", "energy", "能源", "inflation", "通胀"},
		RequiredAny: []string{"oil", "原油", "energy", "能源", "inflation", "通胀"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "geopolitical_trade_realignment", Label: "地缘贸易重组", Scope: "geopolitics"},
		Markers:     []string{"war", "wars", "geopolitical", "tariff", "tariffs", "trade policy", "trade arrangements", "commodities", "global markets", "地缘", "战争", "关税", "贸易", "大宗商品"},
		RequiredAny: []string{"geopolitical", "tariff", "tariffs", "trade policy", "trade arrangements", "commodities", "地缘", "关税", "贸易", "大宗商品"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "ai_credit_contagion", Label: "AI信贷传染", Scope: "credit"},
		Markers:     []string{"ai", "人工智能", "saas", "software", "软件", "cloud", "云端", "data center", "数据中心", "off-balance", "表外", "lease", "租赁", "private credit", "私募信贷", "loan", "贷款", "financing", "融资", "default", "违约", "cash flow", "现金流"},
		RequiredAny: []string{"private credit", "私募信贷", "loan", "贷款", "financing", "融资", "default", "违约"},
		MinScore:    4,
	},
	{
		Meta:        spineFamily{Key: "private_credit_liquidity", Label: "私募信贷流动性", Scope: "credit"},
		Markers:     []string{"private credit", "私募信贷", "redemption", "赎回", "liquidity", "流动性", "markdown", "capital demand"},
		RequiredAny: []string{"private credit", "私募信贷", "redemption", "赎回", "markdown"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "petrodollar_outflow", Label: "石油美元流出", Scope: "geopolitics"},
		Markers:     []string{"petrodollar", "石油美元", "middle east capital", "中东资金", "security credibility", "安全可靠性", "us assets", "美债", "美股"},
		RequiredAny: []string{"petrodollar", "石油美元", "middle east capital", "中东资金"},
		MinScore:    1,
	},
	{
		Meta:        spineFamily{Key: "macro_debt_cycle", Label: "宏观债务周期", Scope: "macro"},
		Markers:     []string{"debt", "债务", "promise", "承诺", "money printing", "印钱", "currency devaluation", "贬值", "financial wealth", "金融财富"},
		RequiredAny: []string{"debt", "债务", "promise", "承诺", "money printing", "印钱"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "ai_power_bottleneck", Label: "AI电力瓶颈", Scope: "tech"},
		Markers:     []string{"ai", "人工智能", "power", "电力", "data center", "数据中心", "cooling", "液冷", "grid", "电网"},
		RequiredAny: []string{"power", "电力", "data center", "数据中心", "cooling", "液冷", "grid", "电网"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "ai_societal_shift", Label: "AI社会影响", Scope: "tech"},
		Markers:     []string{"ai adoption", "artificial intelligence", "人工智能", "societal", "second-order", "third-order", "benefits", "winners", "losers", "社会影响"},
		RequiredAny: []string{"ai adoption", "societal", "second-order", "third-order", "winners", "losers", "社会影响"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "ai_infrastructure_bottleneck", Label: "AI基础设施瓶颈", Scope: "tech"},
		Markers:     []string{"ai", "人工智能", "bottleneck", "瓶颈", "hbm", "memory", "内存", "interconnect", "光模块", "capex"},
		RequiredAny: []string{"hbm", "memory", "内存", "interconnect", "光模块", "capex"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "fed_liquidity_cycle", Label: "美联储流动性周期", Scope: "policy"},
		Markers:     []string{"fed", "美联储", "reserve", "准备金", "tga", "balance sheet", "资产负债表", "liquidity", "流动性"},
		RequiredAny: []string{"fed", "美联储", "reserve", "准备金", "tga", "balance sheet", "资产负债表"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "dollar_regime_shift", Label: "美元制度切换", Scope: "fx"},
		Markers:     []string{"dollar", "美元", "greenback", "real yield", "实际收益率", "regime change", "制度切换"},
		RequiredAny: []string{"dollar", "美元", "greenback", "real yield", "实际收益率"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "gold_inflation_hedge", Label: "黄金通胀对冲", Scope: "asset"},
		Markers:     []string{"gold", "黄金", "inflation hedge", "通胀对冲", "real rate", "实际利率"},
		RequiredAny: []string{"gold", "黄金"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "demographic_fiscal_pressure", Label: "人口财政压力", Scope: "macro"},
		Markers:     []string{"demographic", "aging", "人口老龄化", "税基", "tax base", "fiscal", "财政"},
		RequiredAny: []string{"demographic", "aging", "人口老龄化", "税基", "tax base"},
		MinScore:    2,
	},
	{
		Meta:        spineFamily{Key: "fed_regime_uncertainty", Label: "联储制度不确定性", Scope: "policy"},
		Markers:     []string{"warsh", "沃什", "fed", "联储", "mandate", "沟通", "guidance"},
		RequiredAny: []string{"warsh", "沃什", "fed", "联储"},
		MinScore:    2,
	},
}

func inferSpineFamily(spine PreviewSpine, valid map[string]graphNode) spineFamily {
	text := spineTextForScoring(spine, valid)
	best := spineFamily{}
	bestScore := 0
	for _, rule := range spineFamilyRules {
		if len(rule.RequiredAny) > 0 && !containsAnyText(text, rule.RequiredAny) {
			continue
		}
		score := countMarkers(text, rule.Markers)
		minScore := rule.MinScore
		if minScore <= 0 {
			minScore = 1
		}
		if score < minScore {
			continue
		}
		if score > bestScore {
			best = rule.Meta
			bestScore = score
		}
	}
	if bestScore <= 0 {
		return fallbackSpineFamily(spine)
	}
	return best
}

func countMarkers(text string, markers []string) int {
	count := 0
	for _, marker := range markers {
		if textContainsMarker(text, marker) {
			count++
		}
	}
	return count
}

func textContainsMarker(text, marker string) bool {
	text = strings.ToLower(text)
	marker = strings.ToLower(strings.TrimSpace(marker))
	if marker == "" {
		return false
	}
	if !isSingleASCIIWord(marker) {
		return strings.Contains(text, marker)
	}
	start := 0
	for {
		index := strings.Index(text[start:], marker)
		if index < 0 {
			return false
		}
		pos := start + index
		beforeOK := pos == 0 || !isASCIIWordChar(text[pos-1])
		after := pos + len(marker)
		afterOK := after >= len(text) || !isASCIIWordChar(text[after])
		if beforeOK && afterOK {
			return true
		}
		start = pos + len(marker)
	}
}

func isSingleASCIIWord(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if !isASCIIWordChar(text[i]) {
			return false
		}
	}
	return true
}

func isASCIIWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func fallbackSpineFamily(spine PreviewSpine) spineFamily {
	scope := strings.TrimSpace(spine.Scope)
	if scope == "" {
		scope = strings.TrimSpace(spine.Level)
	}
	if scope == "" {
		scope = "general"
	}
	keySource := strings.TrimSpace(spine.Thesis)
	if keySource == "" {
		keySource = strings.TrimSpace(spine.ID)
	}
	key := fallbackFamilyKey(keySource)
	return spineFamily{
		Key:   key,
		Label: strings.TrimSpace(spine.Thesis),
		Scope: scope,
	}
}

func fallbackFamilyKey(text string) string {
	slug, truncated := slugKey(text)
	digest := shortStableDigest(text)
	if slug == "" {
		return "general_u" + digest
	}
	if truncated {
		return "general_" + slug + "_" + digest
	}
	return "general_" + slug
}

func slugKey(text string) (string, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastUnderscore := false
	truncated := false
	for _, r := range text {
		if b.Len() >= 48 {
			truncated = true
			break
		}
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_"), truncated
}

func shortStableDigest(text string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToLower(strings.TrimSpace(text))))
	return fmt.Sprintf("%08x", hash.Sum32())
}

func enforceSpineBudget(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	if policy.MaxSpines <= 0 || len(spines) <= policy.MaxSpines || policy.PreserveRiskFamilies {
		return renumberSpinePriorities(spines)
	}
	primary := make([]int, 0, 1)
	candidates := make([]int, 0, len(spines))
	for i, spine := range spines {
		if spine.Level == "primary" {
			primary = append(primary, i)
			continue
		}
		candidates = append(candidates, i)
	}
	keep := map[int]struct{}{}
	for _, index := range primary {
		keep[index] = struct{}{}
	}
	remaining := policy.MaxSpines - len(keep)
	if remaining < 0 {
		remaining = 0
	}
	type scoredSpine struct {
		index int
		score float64
	}
	primaryText := spineTextForScoring(primarySpine(spines), valid)
	scored := make([]scoredSpine, 0, len(candidates))
	for _, index := range candidates {
		scored = append(scored, scoredSpine{
			index: index,
			score: summarySpineScore(spines[index], valid, primaryText, policy),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return spines[scored[i].index].Priority < spines[scored[j].index].Priority
	})
	for i := 0; i < remaining && i < len(scored); i++ {
		keep[scored[i].index] = struct{}{}
	}
	out := make([]PreviewSpine, 0, len(keep))
	for i, spine := range spines {
		if _, ok := keep[i]; ok {
			out = append(out, spine)
		}
	}
	return renumberSpinePriorities(out)
}

func primarySpine(spines []PreviewSpine) PreviewSpine {
	for _, spine := range spines {
		if spine.Level == "primary" {
			return spine
		}
	}
	return PreviewSpine{}
}

func summarySpineScore(spine PreviewSpine, valid map[string]graphNode, primaryText string, policy spinePolicy) float64 {
	score := 100.0 - float64(spine.Priority)*2.5
	score += float64(len(spine.Edges)) * 4
	score += float64(len(spine.NodeIDs)) * 1.25
	switch spine.Level {
	case "branch":
		score += 4
	case "local":
		score -= 8
	}
	if spineHasDiscourseRole(spine, valid, "thesis") {
		score += 8
	}
	if spineHasDiscourseRole(spine, valid, "market_move", "implication") {
		score += 6
	}
	if spineHasDiscourseRole(spine, valid, "mechanism") {
		score += 3
	}
	text := spineTextForScoring(spine, valid)
	if policy.ArticleForm == "macro_framework" {
		if summaryTextLooksLocalBehavior(text) {
			score -= 24
		}
		if summaryTextRepeatsPrimaryFamily(text, primaryText) && spine.Priority > 2 {
			score -= 18
		}
	}
	if policy.ArticleForm == "evidence_backed_forecast" {
		if forecastSpineLooksLikeLightSideCaveat(text) {
			score -= 28
		}
		if containsAnyText(text, []string{"货币政策", "monetary policy", "利率", "实际利率", "降息", "美联储", "fed", "financial repression", "金融抑制", "金融压抑"}) {
			score += 12
		}
	}
	return score
}

func forecastSpineLooksLikeLightSideCaveat(text string) bool {
	return containsAnyText(text, []string{"ai", "人工智能"}) &&
		containsAnyText(text, []string{"通胀", "inflation", "反通胀", "disinflation", "deflation"})
}

func spineTextForScoring(spine PreviewSpine, valid map[string]graphNode) string {
	parts := []string{spine.Thesis}
	for _, id := range spine.NodeIDs {
		if node, ok := valid[id]; ok {
			parts = append(parts, node.Text)
		}
	}
	for _, edge := range spine.Edges {
		parts = append(parts, edge.SourceQuote, edge.Reason)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func summaryTextLooksLocalBehavior(text string) bool {
	return containsAnyText(text, []string{
		"emotional trading", "underperform", "investor behavior", "sentiment", "心理", "情绪", "行为",
	})
}

func summaryTextRepeatsPrimaryFamily(text, primaryText string) bool {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(primaryText) == "" {
		return false
	}
	overlap := 0
	for _, family := range macroSummaryAnchorFamilies() {
		if containsAnyText(primaryText, family) && containsAnyText(text, family) {
			overlap++
		}
	}
	return overlap >= 2
}

func macroSummaryAnchorFamilies() [][]string {
	return [][]string{
		{"debt", "债务"},
		{"credit", "信贷", "信用"},
		{"promise", "promises", "承诺", "欠条"},
		{"crisis", "default", "crash", "depression", "危机", "违约", "崩盘"},
		{"money printing", "printed", "货币印刷", "印钱"},
		{"currency devaluation", "devaluation", "贬值"},
		{"financial wealth", "金融财富"},
		{"real wealth", "tangible wealth", "实际财富", "有形财富"},
	}
}

func enforceSinglePrimarySpine(spines []PreviewSpine) []PreviewSpine {
	if len(spines) == 0 {
		return spines
	}
	primaryIndex := -1
	for i := range spines {
		if spines[i].Level != "primary" {
			continue
		}
		if primaryIndex == -1 {
			primaryIndex = i
			continue
		}
		spines[i].Level = "branch"
		if spines[i].Scope == "article" {
			spines[i].Scope = "branch"
		}
	}
	if primaryIndex != -1 {
		return renumberSpinePriorities(spines)
	}
	promoteIndex := 0
	for i := range spines {
		if len(spines[i].Edges) > 0 {
			promoteIndex = i
			break
		}
	}
	spines[promoteIndex].Level = "primary"
	spines[promoteIndex].Scope = "article"
	return renumberSpinePriorities(spines)
}

func compactSpines(spines []PreviewSpine, valid map[string]graphNode) []PreviewSpine {
	if len(spines) < 3 {
		return spines
	}
	sellPressureIndexes := make([]int, 0)
	for i, spine := range spines {
		if spine.Level == "primary" {
			continue
		}
		if isCryptoSellPressureSpine(spine, valid) {
			sellPressureIndexes = append(sellPressureIndexes, i)
		}
	}
	if len(sellPressureIndexes) < 2 {
		return spines
	}
	return mergeSpineIndexes(spines, sellPressureIndexes, "Crypto liquidity / sell-pressure mechanics drive Bitcoin weakness")
}

func isCryptoSellPressureSpine(spine PreviewSpine, valid map[string]graphNode) bool {
	text := strings.ToLower(spine.Thesis)
	for _, id := range spine.NodeIDs {
		if node, ok := valid[id]; ok {
			text += " " + strings.ToLower(node.Text)
		}
	}
	for _, edge := range spine.Edges {
		text += " " + strings.ToLower(edge.SourceQuote) + " " + strings.ToLower(edge.Reason)
	}
	if !containsAnyText(text, []string{"bitcoin", "btc", "比特币", "crypto", "加密"}) {
		return false
	}
	return containsAnyText(text, []string{
		"etf outflow", "etf outflows", "outflow", "outflows",
		"market maker", "market makers", "sell into", "selling pressure", "sell-pressure",
		"stablecoin", "stable coin", "supply contraction", "liquidation", "long liquidation",
		"卖压", "出流", "稳定币", "做市", "清算",
	})
}

func mergeSpineIndexes(spines []PreviewSpine, indexes []int, thesis string) []PreviewSpine {
	indexSet := map[int]struct{}{}
	for _, index := range indexes {
		indexSet[index] = struct{}{}
	}
	first := indexes[0]
	merged := PreviewSpine{
		ID:       spines[first].ID,
		Level:    "branch",
		Priority: spines[first].Priority,
		Thesis:   thesis,
		Scope:    "branch",
	}
	seenNodes := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	for _, index := range indexes {
		for _, id := range spines[index].NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seenNodes[id]; ok {
				continue
			}
			seenNodes[id] = struct{}{}
			merged.NodeIDs = append(merged.NodeIDs, id)
		}
		for _, edge := range spines[index].Edges {
			if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" {
				continue
			}
			key := edge.From + "->" + edge.To
			if _, ok := seenEdges[key]; ok {
				continue
			}
			seenEdges[key] = struct{}{}
			merged.Edges = append(merged.Edges, edge)
		}
	}
	out := make([]PreviewSpine, 0, len(spines)-len(indexes)+1)
	for i, spine := range spines {
		if i == first {
			out = append(out, merged)
			continue
		}
		if _, ok := indexSet[i]; ok {
			continue
		}
		out = append(out, spine)
	}
	for i := range out {
		out[i].Priority = i + 1
	}
	return out
}

func previewEdgeFromGraphEdge(edge graphEdge) PreviewEdge {
	return PreviewEdge{
		From:        edge.From,
		To:          edge.To,
		Kind:        edge.Kind,
		SourceQuote: edge.SourceQuote,
		Reason:      edge.Reason,
	}
}

func normalizePreviewSpineLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "primary", "branch", "local":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "branch"
	}
}

func normalizePreviewSpineScope(value, level string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "article", "section", "paragraph", "branch", "local":
		return strings.ToLower(strings.TrimSpace(value))
	}
	switch level {
	case "primary":
		return "article"
	case "local":
		return "local"
	default:
		return "branch"
	}
}

func normalizePreviewSpinePolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "causal_mechanism", "forecast_inference", "investment_implication", "satirical_analogy", "concept_explanation", "risk_family", "market_update":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func stage4Validate(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState, maxRounds int) (graphState, error) {
	if maxRounds <= 0 {
		return state, nil
	}
	systemPrompt, err := renderStage4SystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	paragraphs := splitParagraphs(bundle.TextContext())
	if len(paragraphs) == 0 {
		return state, nil
	}
	for round := 0; round < maxRounds; round++ {
		totalPatches := 0
		for _, para := range paragraphs {
			var patch struct {
				MissingNodes []struct {
					Text              string `json:"text"`
					SourceQuote       string `json:"source_quote"`
					SuggestedRoleHint string `json:"suggested_role_hint"`
				} `json:"missing_nodes"`
				MissingEdges []struct {
					FromText string `json:"from_text"`
					ToText   string `json:"to_text"`
				} `json:"missing_edges"`
				Misclassified []struct {
					NodeID string `json:"node_id"`
					Issue  string `json:"issue"`
				} `json:"misclassified"`
			}
			userPrompt, err := renderStage4UserPrompt(para, serializeNodeList(state.Nodes), serializeEdgeList(state.Edges))
			if err != nil {
				return graphState{}, err
			}
			if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "validate", &patch); err != nil {
				return graphState{}, err
			}
			totalPatches += len(patch.MissingNodes) + len(patch.MissingEdges) + len(patch.Misclassified)
			state = applyValidatePatch(state, patch)
		}
		var err error
		state, err = stage1Refine(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage1Aggregate(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage2Support(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state = collapseClusters(state)
		state, err = stage3Mainline(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage3Classify(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state.Rounds++
		if totalPatches < 2 {
			break
		}
	}
	return state, nil
}

func stage5Render(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (compile.Output, error) {
	if projected, ok := projectRolesFromSpines(state); ok {
		state = projected
	}
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterTargetNodes(state.Nodes)
	if len(targets) == 0 && len(drivers) > 0 {
		targets = fallbackTargetNodesFromOffGraph(state.OffGraph)
	}
	paths := extractSpinePaths(state)
	if len(paths) == 0 {
		paths = extractPaths(state, drivers, targets)
	}
	paths, satiricalCoveredNodes := applySatiricalRenderProjection(state, paths)
	paths = filterCyclicRenderPaths(paths)
	drivers = mergePathDrivers(drivers, paths)
	targets = mergePathTargets(targets, paths)
	drivers = filterRenderDrivers(drivers, paths)
	targets = filterRenderTargets(targets, paths, state.ArticleForm, satiricalCoveredNodes)
	translated, err := translateAll(ctx, rt, model, uniqueTexts(drivers, targets, paths, state.OffGraph))
	if err != nil {
		return compile.Output{}, err
	}
	cn := func(id, fallback string) string {
		if value, ok := translated[id]; ok && strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	driversOut := make([]string, 0, len(drivers))
	for _, d := range drivers {
		driversOut = append(driversOut, cn(d.ID, d.Text))
	}
	targetsOut := make([]string, 0, len(targets))
	for _, t := range targets {
		targetsOut = append(targetsOut, cn(t.ID, t.Text))
	}
	transmission := make([]compile.TransmissionPath, 0, len(paths))
	for _, p := range paths {
		steps := make([]string, 0, len(p.steps))
		for _, s := range p.steps {
			steps = append(steps, cn(s.ID, s.Text))
		}
		if len(steps) == 0 {
			steps = append(steps, cn(p.driver.ID, p.driver.Text))
		}
		transmission = append(transmission, compile.TransmissionPath{
			Driver: cn(p.driver.ID, p.driver.Text),
			Target: cn(p.target.ID, p.target.Text),
			Steps:  steps,
		})
	}
	evidence, explanation, supplementary := renderOffGraph(state.OffGraph, cn)
	evidence = dedupeStrings(append(evidence, renderSpineIllustrations(state, cn)...))
	summary, err := summarizeChinese(ctx, rt, model, state.ArticleForm, driversOut, targetsOut, transmission, bundle)
	if err != nil {
		summary = fallbackSummary(driversOut, targetsOut)
	}
	if strings.TrimSpace(summary) == "" {
		summary = fallbackSummary(driversOut, targetsOut)
	}
	graph := compile.ReasoningGraph{}
	for _, n := range state.Nodes {
		kind := compile.NodeMechanism
		form := compile.NodeFormObservation
		function := compile.NodeFunctionTransmission
		switch n.Role {
		case roleDriver:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		case roleTransmission:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		}
		if n.IsTarget {
			kind = compile.NodeConclusion
			form = compile.NodeFormJudgment
			function = compile.NodeFunctionClaim
			if n.Ontology == "flow" {
				kind = compile.NodeConclusion
			}
		}
		graph.Nodes = append(graph.Nodes, compile.GraphNode{
			ID:         n.ID,
			Kind:       kind,
			Form:       form,
			Function:   function,
			Text:       cn(n.ID, n.Text),
			OccurredAt: bundle.PostedAt,
		})
	}
	for _, e := range state.Edges {
		graph.Edges = append(graph.Edges, compile.GraphEdge{From: e.From, To: e.To, Kind: compile.EdgePositive})
	}
	return compile.Output{
		Summary:            summary,
		Drivers:            driversOut,
		Targets:            targetsOut,
		TransmissionPaths:  transmission,
		EvidenceNodes:      evidence,
		ExplanationNodes:   explanation,
		SupplementaryNodes: supplementary,
		Graph:              graph,
		Details:            compile.HiddenDetails{Caveats: []string{"compile v2 mvp"}},
		Topics:             nil,
		Confidence:         confidenceFromState(driversOut, targetsOut, transmission),
	}, nil
}

func filterCyclicRenderPaths(paths []renderedPath) []renderedPath {
	if len(paths) < 2 {
		return paths
	}
	reaches := map[string]map[string]struct{}{}
	out := make([]renderedPath, 0, len(paths))
	for _, path := range paths {
		nodeIDs := renderedPathNodeIDs(path)
		if len(nodeIDs) < 2 {
			out = append(out, path)
			continue
		}
		if renderedPathHasCycle(nodeIDs, reaches) {
			continue
		}
		out = append(out, path)
		for i := 0; i+1 < len(nodeIDs); i++ {
			addReachability(reaches, nodeIDs[i], nodeIDs[i+1])
		}
	}
	if len(out) == 0 {
		return paths
	}
	return out
}

func renderedPathNodeIDs(path renderedPath) []string {
	nodeIDs := make([]string, 0, len(path.steps)+2)
	if id := strings.TrimSpace(path.driver.ID); id != "" {
		nodeIDs = append(nodeIDs, id)
	}
	for _, step := range path.steps {
		if id := strings.TrimSpace(step.ID); id != "" {
			nodeIDs = append(nodeIDs, id)
		}
	}
	if id := strings.TrimSpace(path.target.ID); id != "" {
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs
}

func renderedPathHasCycle(nodeIDs []string, reaches map[string]map[string]struct{}) bool {
	seen := map[string]struct{}{}
	for _, id := range nodeIDs {
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
	}
	for i := 0; i+1 < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			if pathReachable(reaches, nodeIDs[j], nodeIDs[i]) {
				return true
			}
		}
	}
	return false
}

func pathReachable(reaches map[string]map[string]struct{}, from, to string) bool {
	if from == to {
		return true
	}
	_, ok := reaches[from][to]
	return ok
}

func addReachability(reaches map[string]map[string]struct{}, from, to string) {
	ensureReachSet := func(id string) map[string]struct{} {
		if reaches[id] == nil {
			reaches[id] = map[string]struct{}{}
		}
		return reaches[id]
	}
	fromSet := ensureReachSet(from)
	fromSet[to] = struct{}{}
	for next := range reaches[to] {
		fromSet[next] = struct{}{}
	}
	for source, targets := range reaches {
		if source == from {
			continue
		}
		if _, ok := targets[from]; !ok {
			continue
		}
		targets[to] = struct{}{}
		for next := range reaches[to] {
			targets[next] = struct{}{}
		}
	}
}

type satiricalProjection struct {
	path    renderedPath
	nodeSet map[string]struct{}
}

func applySatiricalRenderProjection(state graphState, paths []renderedPath) ([]renderedPath, map[string]struct{}) {
	if len(state.Spines) == 0 {
		return paths, nil
	}
	nodeIndex := map[string]graphNode{}
	valid := map[string]struct{}{}
	for _, node := range state.Nodes {
		nodeIndex[node.ID] = node
		valid[node.ID] = struct{}{}
	}
	projections := make([]satiricalProjection, 0)
	for _, spine := range state.Spines {
		if normalizePreviewSpinePolicy(spine.Policy) != "satirical_analogy" {
			continue
		}
		path, nodeSet, ok := satiricalDisplayPath(spine, nodeIndex, valid, state.OffGraph)
		if !ok {
			continue
		}
		projections = append(projections, satiricalProjection{path: path, nodeSet: nodeSet})
	}
	if len(projections) == 0 {
		return paths, nil
	}
	covered := map[string]struct{}{}
	out := make([]renderedPath, 0, len(projections)+len(paths))
	for _, projection := range projections {
		out = append(out, projection.path)
		for id := range projection.nodeSet {
			covered[id] = struct{}{}
		}
	}
	for _, path := range paths {
		if pathWithinAnySatiricalProjection(path, projections) {
			continue
		}
		out = append(out, path)
	}
	return out, covered
}

func satiricalDisplayPath(spine PreviewSpine, nodes map[string]graphNode, valid map[string]struct{}, offGraph []offGraphItem) (renderedPath, map[string]struct{}, bool) {
	nodeIDs := validSpineNodeIDs(spine, valid)
	if len(nodeIDs) < 2 {
		return renderedPath{}, nil, false
	}
	nodeSet := map[string]struct{}{}
	ordered := make([]graphNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		node, ok := nodes[id]
		if !ok {
			continue
		}
		nodeSet[id] = struct{}{}
		ordered = append(ordered, node)
	}
	if len(ordered) < 2 {
		return renderedPath{}, nil, false
	}
	driver, driverOK := bestSatiricalDriverNode(ordered)
	target, targetOK := bestSatiricalTargetNode(ordered, driver.ID)
	if offTarget, ok := bestSatiricalOffGraphTarget(offGraph, nodeSet); ok && (!targetOK || satiricalTargetScore(offTarget) > satiricalTargetScore(target)) {
		target = offTarget
		targetOK = true
	}
	if !driverOK || !targetOK || driver.ID == target.ID {
		return renderedPath{}, nil, false
	}
	steps := make([]graphNode, 0, min(4, max(0, len(ordered)-2)))
	for _, node := range ordered {
		if node.ID == driver.ID || node.ID == target.ID {
			continue
		}
		steps = append(steps, node)
		if len(steps) >= 4 {
			break
		}
	}
	return renderedPath{driver: driver, target: target, steps: steps}, nodeSet, true
}

func bestSatiricalDriverNode(nodes []graphNode) (graphNode, bool) {
	best := graphNode{}
	bestScore := -999
	for _, node := range nodes {
		score := satiricalDriverScore(node)
		if score > bestScore {
			best = node
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func bestSatiricalTargetNode(nodes []graphNode, driverID string) (graphNode, bool) {
	best := graphNode{}
	bestScore := -999
	for _, node := range nodes {
		if node.ID == driverID {
			continue
		}
		score := satiricalTargetScore(node)
		if score > bestScore {
			best = node
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func bestSatiricalOffGraphTarget(offGraph []offGraphItem, spineNodeSet map[string]struct{}) (graphNode, bool) {
	best := graphNode{}
	bestScore := -999
	for i, item := range offGraph {
		attachTo := strings.TrimSpace(item.AttachesTo)
		if attachTo == "" {
			continue
		}
		if _, ok := spineNodeSet[attachTo]; !ok {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("satire_offgraph_target_%d", i+1)
		}
		node := graphNode{
			ID:            id,
			Text:          text,
			SourceQuote:   item.SourceQuote,
			DiscourseRole: item.Role,
			Role:          roleTransmission,
			IsTarget:      true,
		}
		score := satiricalTargetScore(node)
		if score > bestScore {
			best = node
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func satiricalDriverScore(node graphNode) int {
	text := strings.ToLower(strings.TrimSpace(node.Text))
	score := 0
	switch normalizeDiscourseRole(node.DiscourseRole) {
	case "satire_target":
		score += 35
	case "implied_thesis":
		score += 30
	case "thesis":
		score += 18
	case "mechanism":
		score += 5
	}
	for _, marker := range []string{"叙事", "包装", "公平", "不公平", "牌照", "表面", "实质", "控制", "机制", "忽悠", "手续费", "零售客户", "买单"} {
		if strings.Contains(text, marker) {
			score += 7
		}
	}
	for _, marker := range []string{"2000", "每人", "每月", "年息", "委托贷款", "抽一人", "中奖者", "存银行"} {
		if strings.Contains(text, marker) {
			score -= 10
		}
	}
	return score
}

func satiricalTargetScore(node graphNode) int {
	text := strings.ToLower(strings.TrimSpace(node.Text))
	score := 0
	switch normalizeDiscourseRole(node.DiscourseRole) {
	case "implication":
		score += 18
	case "market_move":
		score += 10
	}
	for _, marker := range []string{"归管理者", "归我", "基金", "净亏", "承担", "成本", "缺口", "后75", "零售客户", "买单", "损失", "亏", "转移", "锁定", "无法取出"} {
		if strings.Contains(text, marker) {
			score += 8
		}
	}
	for _, marker := range []string{"规则", "每人", "每月", "年息", "叙事", "包装"} {
		if strings.Contains(text, marker) {
			score -= 6
		}
	}
	return score
}

func pathWithinAnySatiricalProjection(path renderedPath, projections []satiricalProjection) bool {
	for _, projection := range projections {
		if _, ok := projection.nodeSet[path.driver.ID]; !ok {
			continue
		}
		if _, ok := projection.nodeSet[path.target.ID]; ok {
			return true
		}
	}
	return false
}

func renderSpineIllustrations(state graphState, cn func(string, string) string) []string {
	if len(state.Spines) == 0 {
		return nil
	}
	byID := map[string]graphNode{}
	for _, node := range state.Nodes {
		byID[node.ID] = node
	}
	out := make([]string, 0)
	for _, spine := range state.Spines {
		if normalizePreviewSpinePolicy(spine.Policy) != "satirical_analogy" {
			continue
		}
		for _, id := range spine.NodeIDs {
			node, ok := byID[id]
			if ok && normalizeDiscourseRole(node.DiscourseRole) == "analogy" {
				out = append(out, cn(node.ID, node.Text))
			}
		}
		for _, edge := range spine.Edges {
			if !isIllustrationKind(edge.Kind) {
				continue
			}
			node, ok := byID[edge.From]
			if !ok {
				continue
			}
			out = append(out, cn(node.ID, node.Text))
		}
	}
	return out
}

func mergePathDrivers(drivers []graphNode, paths []renderedPath) []graphNode {
	out := append([]graphNode(nil), drivers...)
	seen := map[string]struct{}{}
	for _, driver := range out {
		seen[driver.ID] = struct{}{}
	}
	for _, path := range paths {
		if strings.TrimSpace(path.driver.ID) == "" {
			continue
		}
		if _, ok := seen[path.driver.ID]; ok {
			continue
		}
		seen[path.driver.ID] = struct{}{}
		out = append(out, path.driver)
	}
	return out
}

func mergePathTargets(targets []graphNode, paths []renderedPath) []graphNode {
	out := append([]graphNode(nil), targets...)
	seen := map[string]struct{}{}
	for _, target := range out {
		seen[target.ID] = struct{}{}
	}
	for _, path := range paths {
		if strings.TrimSpace(path.target.ID) == "" {
			continue
		}
		if _, ok := seen[path.target.ID]; ok {
			continue
		}
		seen[path.target.ID] = struct{}{}
		target := path.target
		target.IsTarget = true
		out = append(out, target)
	}
	return out
}

func filterRenderDrivers(drivers []graphNode, paths []renderedPath) []graphNode {
	if len(drivers) == 0 || len(paths) == 0 {
		return drivers
	}
	pathTargets := map[string]struct{}{}
	pathSteps := map[string]struct{}{}
	for _, path := range paths {
		if strings.TrimSpace(path.target.ID) != "" {
			pathTargets[path.target.ID] = struct{}{}
		}
		for _, step := range path.steps {
			if strings.TrimSpace(step.ID) != "" {
				pathSteps[step.ID] = struct{}{}
			}
		}
	}
	out := make([]graphNode, 0, len(drivers))
	for _, driver := range drivers {
		if _, ok := pathTargets[driver.ID]; ok {
			continue
		}
		if _, ok := pathSteps[driver.ID]; ok {
			continue
		}
		out = append(out, driver)
	}
	if len(out) == 0 {
		return drivers
	}
	return out
}

func filterRenderTargets(targets []graphNode, paths []renderedPath, articleForm string, satiricalCoveredNodes map[string]struct{}) []graphNode {
	if len(targets) == 0 || len(paths) == 0 {
		return targets
	}
	pathDrivers := map[string]struct{}{}
	pathSteps := map[string]struct{}{}
	for _, path := range paths {
		if strings.TrimSpace(path.driver.ID) != "" {
			pathDrivers[path.driver.ID] = struct{}{}
		}
		for _, step := range path.steps {
			if strings.TrimSpace(step.ID) != "" {
				pathSteps[step.ID] = struct{}{}
			}
		}
	}
	out := make([]graphNode, 0, len(targets))
	for _, target := range targets {
		if _, ok := pathDrivers[target.ID]; ok {
			continue
		}
		if _, ok := pathSteps[target.ID]; ok && hasID(satiricalCoveredNodes, target.ID) {
			continue
		}
		if isRenderProcessStateTarget(target) {
			continue
		}
		if isLowWeightForecastTarget(target, articleForm) {
			continue
		}
		out = append(out, target)
	}
	if len(out) == 0 {
		return targets
	}
	return out
}

func isLowWeightForecastTarget(node graphNode, articleForm string) bool {
	if normalizeArticleForm(articleForm) != "evidence_backed_forecast" {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(node.Text))
	if text == "" {
		return false
	}
	if containsAnyText(text, []string{"ai", "人工智能"}) &&
		containsAnyText(text, []string{"通胀", "inflation", "反通胀", "disinflation", "deflation"}) {
		return true
	}
	if containsAnyText(text, []string{"跨境", "税务", "税负", "pfic", "tax", "jurisdiction"}) {
		return true
	}
	return normalizeDiscourseRole(node.DiscourseRole) == "caveat"
}

func isRenderProcessStateTarget(node graphNode) bool {
	text := strings.ToLower(strings.TrimSpace(node.Text))
	if text == "" {
		return false
	}
	if containsAnyText(text, []string{"核心机制", "core mechanism", "机制是", "mechanism is"}) {
		return true
	}
	if containsAnyText(text, []string{"金融抑制", "financial repression"}) &&
		containsAnyText(text, []string{"存款利率上限", "资本管制", "锁定资金", "购买国债", "capital control", "rate cap"}) {
		return true
	}
	hasProcessSubject := containsAnyText(text, []string{
		"金融抑制", "financial repression", "机制", "制度", "regime",
	})
	if !hasProcessSubject {
		return false
	}
	return containsAnyText(text, []string{
		"启动", "开启", "正式开启", "正式启动", "launch", "starts", "begins", "triggered",
	})
}

func fallbackTargetNodesFromOffGraph(off []offGraphItem) []graphNode {
	type candidate struct {
		node  graphNode
		score int
	}
	candidates := make([]candidate, 0)
	for i, item := range off {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		score := fallbackTargetScore(text)
		if score <= 0 {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("fallback_target_%d", i+1)
		}
		candidates = append(candidates, candidate{
			node: graphNode{
				ID:       id,
				Text:     text,
				Role:     roleTransmission,
				Ontology: inferTargetKind(text, true),
				IsTarget: true,
			},
			score: score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}
	out := make([]graphNode, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, candidates[i].node)
	}
	return out
}

func fallbackTargetScore(text string) int {
	lower := strings.ToLower(strings.TrimSpace(text))
	score := 0
	for _, marker := range []string{
		"风险", "压力", "挤兑", "危机", "承压", "下降", "流出", "减少",
		"重估", "上升", "紧张", "爆发", "违约", "撤资", "赎回", "系统性",
		"run", "stress", "risk", "pressure", "outflow", "redemption", "default",
	} {
		if strings.Contains(lower, marker) {
			score += 2
		}
	}
	for _, marker := range []string{"私募信贷", "美债", "美股", "拥挤交易", "资金", "流动性"} {
		if strings.Contains(lower, marker) {
			score++
		}
	}
	for _, marker := range []string{"指", "本质", "形成", "怎么回事", "不受银行监管"} {
		if strings.Contains(lower, marker) {
			score -= 2
		}
	}
	return score
}

func applyValidatePatch(state graphState, patch struct {
	MissingNodes []struct {
		Text              string `json:"text"`
		SourceQuote       string `json:"source_quote"`
		SuggestedRoleHint string `json:"suggested_role_hint"`
	} `json:"missing_nodes"`
	MissingEdges []struct {
		FromText string `json:"from_text"`
		ToText   string `json:"to_text"`
	} `json:"missing_edges"`
	Misclassified []struct {
		NodeID string `json:"node_id"`
		Issue  string `json:"issue"`
	} `json:"misclassified"`
}) graphState {
	nextNode := len(state.Nodes) + 1
	textToID := map[string]string{}
	for _, n := range state.Nodes {
		textToID[normalizeText(n.Text)] = n.ID
	}
	for _, item := range patch.MissingNodes {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		key := normalizeText(text)
		if _, ok := textToID[key]; ok {
			continue
		}
		id := fmt.Sprintf("n%d", nextNode)
		nextNode++
		state.Nodes = append(state.Nodes, graphNode{ID: id, Text: text, SourceQuote: strings.TrimSpace(item.SourceQuote)})
		textToID[key] = id
	}
	for _, item := range patch.MissingEdges {
		fromID := textToID[normalizeText(item.FromText)]
		toID := textToID[normalizeText(item.ToText)]
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		if !hasEdge(state.Edges, fromID, toID) {
			state.Edges = append(state.Edges, graphEdge{From: fromID, To: toID})
		}
	}
	for _, item := range patch.Misclassified {
		if strings.TrimSpace(item.NodeID) == "" {
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:         fmt.Sprintf("mis_%s", item.NodeID),
			Text:       strings.TrimSpace(item.Issue),
			Role:       "supplementary",
			AttachesTo: strings.TrimSpace(item.NodeID),
		})
	}
	return state
}

type renderedPath struct {
	driver graphNode
	target graphNode
	steps  []graphNode
}

func extractSpinePaths(state graphState) []renderedPath {
	if len(state.Spines) == 0 {
		return nil
	}
	valid := map[string]struct{}{}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		valid[node.ID] = struct{}{}
		nodeIndex[node.ID] = node
	}
	out := make([]renderedPath, 0)
	seen := map[string]struct{}{}
	for _, spine := range state.Spines {
		nodeIDs := validSpineNodeIDs(spine, valid)
		if len(nodeIDs) < 2 {
			continue
		}
		sources, terminals := spineSourceAndTerminalIDs(spine, nodeIDs, valid, nodeIndex)
		adj := spineAdjacency(spine, valid, nodeIndex)
		if len(adj) == 0 && len(nodeIDs) >= 2 {
			for i := 0; i+1 < len(nodeIDs); i++ {
				adj[nodeIDs[i]] = append(adj[nodeIDs[i]], nodeIDs[i+1])
			}
		}
		for _, source := range sources {
			for _, terminal := range terminals {
				pathIDs := shortestPath(adj, source, terminal)
				if len(pathIDs) < 2 {
					continue
				}
				key := strings.Join(pathIDs, "->")
				if _, ok := seen[key]; ok {
					continue
				}
				driver, ok := nodeByID(state.Nodes, source)
				if !ok {
					continue
				}
				target, ok := nodeByID(state.Nodes, terminal)
				if !ok {
					continue
				}
				steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
				for _, id := range pathIDs[1 : len(pathIDs)-1] {
					if node, ok := nodeByID(state.Nodes, id); ok {
						steps = append(steps, node)
					}
				}
				seen[key] = struct{}{}
				out = append(out, renderedPath{driver: driver, target: target, steps: steps})
			}
		}
	}
	return out
}

func spineAdjacency(spine PreviewSpine, valid map[string]struct{}, nodes map[string]graphNode) map[string][]string {
	adj := map[string][]string{}
	for _, edge := range spineProjectionEdges(spine, nodes) {
		if _, ok := valid[edge.From]; !ok {
			continue
		}
		if _, ok := valid[edge.To]; !ok {
			continue
		}
		if edge.From == edge.To {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	return adj
}

func extractPaths(state graphState, drivers, targets []graphNode) []renderedPath {
	adj := map[string][]string{}
	for _, e := range state.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	var out []renderedPath
	for _, d := range drivers {
		for _, t := range targets {
			pathIDs := shortestPath(adj, d.ID, t.ID)
			if len(pathIDs) < 2 {
				continue
			}
			steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
			for _, id := range pathIDs[1 : len(pathIDs)-1] {
				if node, ok := nodeByID(state.Nodes, id); ok {
					steps = append(steps, node)
				}
			}
			out = append(out, renderedPath{driver: d, target: t, steps: steps})
		}
	}
	return out
}

func hasEdge(edges []graphEdge, from, to string) bool {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func shortestPath(adj map[string][]string, start, target string) []string {
	type item struct {
		id   string
		path []string
	}
	queue := []item{{id: start, path: []string{start}}}
	seen := map[string]struct{}{start: {}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == target {
			return cur.path
		}
		for _, next := range adj[cur.id] {
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, item{id: next, path: appendPathNode(cur.path, next)})
		}
	}
	return nil
}

func appendPathNode(path []string, next string) []string {
	cloned := compile.CloneStrings(path)
	return append(cloned, next)
}

func dedupeEdges(edges []graphEdge) []graphEdge {
	seen := map[string]struct{}{}
	out := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		key := e.From + "->" + e.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out
}

func pruneTransitiveRelations(edges []graphEdge) []graphEdge {
	edges = dedupeEdges(edges)
	out := make([]graphEdge, 0, len(edges))
	for i, edge := range edges {
		if hasAlternateMainlinePath(edges, i, edge.From, edge.To) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func hasAlternateMainlinePath(edges []graphEdge, skipIndex int, from, to string) bool {
	adj := map[string][]string{}
	for i, edge := range edges {
		if i == skipIndex {
			continue
		}
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	path := shortestPath(adj, from, to)
	return len(path) >= 3
}

func dedupeAuxEdges(edges []auxEdge) []auxEdge {
	seen := map[string]struct{}{}
	out := make([]auxEdge, 0, len(edges))
	for _, edge := range edges {
		key := edge.Kind + "|" + edge.From + "|" + edge.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, edge)
	}
	return out
}

func buildAuxEdgesFromSupport(nodes []graphNode, raw []struct {
	From        string `json:"from"`
	To          string `json:"to"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}) []auxEdge {
	valid := map[string]struct{}{}
	for _, n := range nodes {
		valid[n.ID] = struct{}{}
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		out = append(out, auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        "evidence",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	return out
}

func buildAuxEdgesFromExplanation(nodes []graphNode, raw []struct {
	From        string `json:"from"`
	To          string `json:"to"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}) []auxEdge {
	valid := map[string]struct{}{}
	for _, n := range nodes {
		valid[n.ID] = struct{}{}
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		out = append(out, auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        "explanation",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	return out
}

func buildAuxEdgesFromSupportEdges(nodes []graphNode, raw []supportEdgePatch) []auxEdge {
	valid := map[string]graphNode{}
	for _, n := range nodes {
		valid[n.ID] = n
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		fromNode, ok := valid[e.From]
		if !ok {
			continue
		}
		toNode, ok := valid[e.To]
		if !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		kind, ok := normalizeSupportKind(e.Kind)
		if !ok {
			continue
		}
		edge := auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        kind,
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		}
		if isLikelyMainlineAuxEdge(edge, fromNode, toNode) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func isLikelyMainlineAuxEdge(edge auxEdge, fromNode, toNode graphNode) bool {
	switch strings.TrimSpace(edge.Kind) {
	case "explanation", "supplementary":
	default:
		return false
	}
	if looksLikeAuxiliaryDetailNode(fromNode.Text) {
		return false
	}
	if !looksLikeOutcomeOrProcessEndpoint(fromNode.Text) || !looksLikeOutcomeOrProcessEndpoint(toNode.Text) {
		return false
	}
	context := strings.ToLower(strings.Join([]string{
		fromNode.Text,
		toNode.Text,
		fromNode.SourceQuote,
		toNode.SourceQuote,
		edge.SourceQuote,
		edge.Reason,
	}, " "))
	return containsAnyText(context, supportDriveMarkers())
}

func looksLikeAuxiliaryDetailNode(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if looksLikeOutcomeOrProcessEndpoint(text) && !containsAnyText(lower, []string{"赎回申请", "赎回请求", "机构资金", "占比", "比例", "不良贷款"}) {
		return false
	}
	if looksLikePureQuantOrThreshold(lower) || looksLikePureRuleOrLimit(lower) {
		return true
	}
	for _, marker := range []string{
		"底层资产", "企业贷款", "日常流动性", "机构资金", "机构资金占比", "贷款标准", "估值透明度", "pik", "不良贷款", "赎回申请", "赎回请求", "国防预算", "defense budget",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeOutcomeOrProcessEndpoint(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if looksLikeSubjectChangeNode(text) || looksLikeConcreteBranchResult(lower) {
		return true
	}
	for _, marker := range []string{
		"转冷", "转向", "抛售", "被抛售", "收缩", "飙升", "回落", "被推高", "高企", "维持高位", "支出上升", "被挤压", "形成", "受影响", "被压低", "被拖累", "成本上升", "居高不下", "flight to cash", "现金为王", "现象出现",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func supportDriveMarkers() []string {
	return []string{
		"导致", "引发", "造成", "使", "使得", "影响", "推高", "推动", "压低", "拖累", "传导", "形成", "收缩", "飙升", "解释为什么", "因此", "然后",
		"cause", "causes", "caused", "lead to", "leads to", "led to", "trigger", "triggers", "triggered", "push", "pushes", "pushed", "drives", "driven", "forms", "formed", "creates", "created", "explains why", "consequence", "therefore", "then", "which leads",
	}
}

func normalizeSupportKind(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "evidence":
		return "evidence", true
	case "inference", "inferential", "proof":
		return "inference", true
	case "explanation":
		return "explanation", true
	case "supplement", "supplementary":
		return "supplementary", true
	default:
		return "", false
	}
}

func normalizeMainlineRelationKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "inference", "inferential", "proof":
		return "inference"
	case "illustration", "analogy", "satire", "satirical":
		return "illustration"
	default:
		return "causal"
	}
}

func auxNodeRole(edge auxEdge, nodeID string) (string, bool) {
	switch edge.Kind {
	case "evidence", "inference":
		if edge.From == nodeID {
			return edge.Kind, true
		}
	case "explanation":
		if edge.From == nodeID {
			return "explanation", true
		}
	case "supplementary":
		if edge.From == nodeID {
			return "supplementary", true
		}
	}
	return "", false
}

func collectAuxComponent(adj map[string][]string, start string, visited map[string]struct{}) []string {
	stack := []string{start}
	component := make([]string, 0)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		component = append(component, id)
		stack = append(stack, adj[id]...)
	}
	return component
}

func chooseClusterHead(component []string, edges []auxEdge, nodeIndex map[string]graphNode) string {
	if len(component) == 0 {
		return ""
	}
	member := map[string]struct{}{}
	for _, id := range component {
		member[id] = struct{}{}
	}
	inScore := map[string]float64{}
	outScore := map[string]float64{}
	inCount := map[string]int{}
	outCount := map[string]int{}
	for _, edge := range edges {
		if _, ok := member[edge.From]; !ok {
			continue
		}
		if _, ok := member[edge.To]; !ok {
			continue
		}
		w := auxEdgeWeight(edge.Kind)
		outScore[edge.From] += w
		inScore[edge.To] += w
		outCount[edge.From]++
		inCount[edge.To]++
	}
	candidates := make([]string, 0, len(component))
	for _, candidate := range component {
		// A support edge means `from` is serving another node, so it cannot be
		// the component core. If the model creates a cycle, fall back below.
		if outCount[candidate] == 0 {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		candidates = component
	}
	best := candidates[0]
	bestScore := clusterHeadScore(best, inScore, outScore, nodeIndex)
	bestTie := clusterHeadTieBreak(nodeIndex[best].Text)
	for _, candidate := range candidates[1:] {
		score := clusterHeadScore(candidate, inScore, outScore, nodeIndex)
		tie := clusterHeadTieBreak(nodeIndex[candidate].Text)
		switch {
		case score > bestScore:
			best = candidate
			bestScore = score
			bestTie = tie
		case score == bestScore && inCount[candidate] > inCount[best]:
			best = candidate
			bestTie = tie
		case score == bestScore && inCount[candidate] == inCount[best] && outCount[candidate] < outCount[best]:
			best = candidate
			bestTie = tie
		case score == bestScore && inCount[candidate] == inCount[best] && outCount[candidate] == outCount[best] && tie > bestTie:
			best = candidate
			bestTie = tie
		}
	}
	return best
}

func auxEdgeWeight(kind string) float64 {
	switch strings.TrimSpace(kind) {
	case "evidence":
		return 3.0
	case "inference":
		return 3.25
	case "explanation":
		return 2.0
	case "supplementary":
		return 2.5
	default:
		return 1.0
	}
}

func canonicalityScore(nodeID string, inScore, outScore map[string]float64) float64 {
	return inScore[nodeID] - outScore[nodeID]
}

func clusterHeadScore(nodeID string, inScore, outScore map[string]float64, nodeIndex map[string]graphNode) float64 {
	node := nodeIndex[nodeID]
	return canonicalityScore(nodeID, inScore, outScore) + discourseRoleHeadBoost(node.DiscourseRole) + 0.35*clusterHeadTieBreak(node.Text)
}

func discourseRolePriority(role string) int {
	switch normalizeDiscourseRole(role) {
	case "thesis":
		return 7
	case "mechanism":
		return 6
	case "implication":
		return 5
	case "market_move":
		return 4
	case "caveat":
		return 3
	case "evidence":
		return 2
	case "example":
		return 1
	default:
		return 0
	}
}

func discourseRoleHeadBoost(role string) float64 {
	switch normalizeDiscourseRole(role) {
	case "thesis":
		return 8.0
	case "mechanism":
		return 5.0
	case "implication":
		return 3.0
	case "market_move":
		return 2.0
	case "caveat":
		return -1.0
	case "evidence":
		return -3.0
	case "example":
		return -4.0
	default:
		return 0
	}
}

func clusterHeadTieBreak(text string) float64 {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	score := 0.0
	if looksLikeSubjectChangeNode(text) {
		score += 4.0
	}
	if looksLikeConcreteBranchResult(lower) {
		score += 2.5
	}
	if looksLikePureQuantOrThreshold(lower) {
		score -= 3.0
	}
	if looksLikePureRuleOrLimit(lower) {
		score -= 3.5
	}
	if looksLikeBroadCommentary(lower) {
		score -= 2.5
	}
	if looksLikeForecastOrDominoFraming(lower) {
		score -= 2.5
	}
	if looksLikeProcessSummary(lower) {
		score -= 2.0
	}
	return score
}

func isEligibleTargetHead(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if !looksLikeSubjectChangeNode(text) && !looksLikeConcreteBranchResult(lower) {
		return false
	}
	if looksLikePureQuantOrThreshold(lower) {
		return false
	}
	if looksLikePureRuleOrLimit(lower) {
		return false
	}
	if looksLikeBroadCommentary(lower) {
		return false
	}
	if looksLikeForecastOrDominoFraming(lower) {
		return false
	}
	if looksLikeProcessSummary(lower) {
		return false
	}
	return true
}

func looksLikeConcreteBranchResult(lower string) bool {
	for _, marker := range []string{"危机", "爆雷", "受阻", "上涨", "下跌", "承压", "压力", "流入减少", "流出", "减少", "流动性", "锁定", "短缺", "重洗牌", "恶化", "松动", "冻结", "挤兑", "挤提", "集中赎回", "赎回潮", "恐慌性赎回", "坏账风险", "违约风险", "下跌风险", "爆发概率", "危机爆发", "受限", "风险上升", "概率上升", "下行风险", "上涨", "下降", "spike", "surge", "freeze", "run", "shortage", "squeeze", "outflow", "pressure"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeSubjectChangeNode(text string) bool {
	for _, marker := range []string{"上涨", "下跌", "上升", "下降", "减少", "收缩", "扩张", "恶化", "改善", "爆雷", "危机", "受阻", "承压", "压力", "流入减少", "流出", "飙升", "流动性", "锁定", "挤压", "挤兑", "挤提", "集中赎回", "赎回潮", "恐慌性赎回", "坏账风险", "违约风险", "下跌风险", "爆发概率", "危机爆发", "松动", "上涨", "rises", "falls", "surges", "drops", "faces", "suffers", "outflow", "pressure"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikePureQuantOrThreshold(lower string) bool {
	for _, marker := range []string{"%", "亿", "万亿", "上限", "仅", "达到", "4.999", "44.3", "11.3", "21.9", "15.7"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikePureRuleOrLimit(lower string) bool {
	for _, marker := range []string{"最多", "上限", "允许", "规则", "每季度", "limit", "allows"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeBroadCommentary(lower string) bool {
	for _, marker := range []string{"底色", "局面", "更棘手", "气氛", "评论", "整体", "复杂", "流动性环境", "headline", "hook", "时代", "并列", "系统性问题"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeForecastOrDominoFraming(lower string) bool {
	for _, marker := range []string{"可能", "未必", "first domino", "domino", "最先", "判断", "预测", "预计"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeProcessSummary(lower string) bool {
	for _, marker := range []string{"形成", "螺旋", "交织", "拖住", "推高", "挤压", "连锁", "一层层", "重塑", "summary"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func collectBranchMainlineNodes(edges []graphEdge, branchHeads []string) map[string]struct{} {
	keep := map[string]struct{}{}
	reverse := map[string][]string{}
	for _, edge := range edges {
		reverse[edge.To] = append(reverse[edge.To], edge.From)
	}
	stack := append([]string(nil), branchHeads...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := keep[id]; ok {
			continue
		}
		keep[id] = struct{}{}
		stack = append(stack, reverse[id]...)
	}
	return keep
}

func chooseBranchAttachment(edges []graphEdge, nodeID string, keep map[string]struct{}, branchHeads []string) string {
	for _, edge := range edges {
		if edge.From == nodeID {
			if _, ok := keep[edge.To]; ok {
				return edge.To
			}
		}
	}
	for _, edge := range edges {
		if edge.To == nodeID {
			if _, ok := keep[edge.From]; ok {
				return edge.From
			}
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func filterNodesByRole(nodes []graphNode, role graphRole) []graphNode {
	out := make([]graphNode, 0)
	for _, n := range nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

func filterTargetNodes(nodes []graphNode) []graphNode {
	out := make([]graphNode, 0)
	for _, n := range nodes {
		if n.IsTarget {
			out = append(out, n)
		}
	}
	return out
}

func predecessorOf(edges []graphEdge, id string) string {
	for _, e := range edges {
		if e.To == id {
			return e.From
		}
	}
	return ""
}

func predecessorTexts(state graphState, id string) []string {
	out := make([]string, 0)
	for _, edge := range state.Edges {
		if edge.To != id {
			continue
		}
		if node, ok := nodeByID(state.Nodes, edge.From); ok {
			out = append(out, node.Text)
		}
	}
	return out
}

func successorTexts(state graphState, id string) []string {
	out := make([]string, 0)
	for _, edge := range state.Edges {
		if edge.From != id {
			continue
		}
		if node, ok := nodeByID(state.Nodes, edge.To); ok {
			out = append(out, node.Text)
		}
	}
	return out
}

func serializeNeighborTexts(values []string) string {
	values = compile.CloneStrings(values)
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out = append(out, "- "+value)
	}
	if len(out) == 0 {
		return "- (none)"
	}
	return strings.Join(out, "\n")
}

func nodeByID(nodes []graphNode, id string) (graphNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return graphNode{}, false
}

func normalizeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func uniqueTexts(nodes []graphNode, targets []graphNode, paths []renderedPath, off []offGraphItem) []map[string]string {
	seen := map[string]struct{}{}
	out := make([]map[string]string, 0)
	add := func(id, text string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, map[string]string{"id": id, "text": text})
	}
	for _, n := range nodes {
		add(n.ID, n.Text)
	}
	for _, n := range targets {
		add(n.ID, n.Text)
	}
	for _, p := range paths {
		add(p.driver.ID, p.driver.Text)
		add(p.target.ID, p.target.Text)
		for _, s := range p.steps {
			add(s.ID, s.Text)
		}
	}
	for _, o := range off {
		add(o.ID, o.Text)
	}
	return out
}

func translateAll(ctx context.Context, rt runtimeChat, model string, items []map[string]string) (map[string]string, error) {
	if len(items) == 0 {
		return map[string]string{}, nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	bundle := compile.Bundle{UnitID: "translate", Source: "compilev2", ExternalID: "translate", Content: string(payload)}
	var result struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	systemPrompt, err := renderStage5TranslateSystemPrompt()
	if err != nil {
		return nil, err
	}
	userPrompt, err := renderStage5TranslateUserPrompt(string(payload))
	if err != nil {
		return nil, err
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "translate", &result); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, item := range result.Translations {
		out[item.ID] = strings.TrimSpace(item.Text)
	}
	return out, nil
}

func summarizeChinese(ctx context.Context, rt runtimeChat, model string, articleForm string, drivers, targets []string, paths []compile.TransmissionPath, bundle compile.Bundle) (string, error) {
	payload, err := json.Marshal(map[string]any{"article_form": normalizeArticleForm(articleForm), "drivers": drivers, "targets": targets, "paths": paths})
	if err != nil {
		return "", err
	}
	var result struct {
		Summary string `json:"summary"`
	}
	systemPrompt, err := renderStage5SummarySystemPrompt()
	if err != nil {
		return "", err
	}
	userPrompt, err := renderStage5SummaryUserPrompt(string(payload))
	if err != nil {
		return "", err
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "summary", &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Summary), nil
}

func stageJSONCall(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, systemPrompt string, userPrompt string, stageName string, target any) error {
	req, err := compile.BuildProviderRequest(stageModel(stageName, model), bundle, systemPrompt, userPrompt, stageSearch(stageName))
	if err != nil {
		return err
	}
	req.JSONSchema = stageJSONSchema(stageName)
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return err
	}
	if err := parseJSONObject(resp.Text, target); err != nil {
		return fmt.Errorf("%s parse: %w", stageName, err)
	}
	return nil
}

func stageJSONSchema(stageName string) *llm.Schema {
	switch strings.TrimSpace(stageName) {
	case "extract":
		return &llm.Schema{
			Name:     "compile_extract",
			Required: []string{"article_form", "nodes", "off_graph"},
			Properties: map[string]any{
				"article_form": map[string]any{"type": "string"},
				"nodes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text", "source_quote", "role"},
						"properties": map[string]any{
							"id":           map[string]any{"type": "string"},
							"text":         map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
							"role":         map[string]any{"type": "string"},
						},
					},
				},
				"off_graph": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text", "role", "attaches_to", "source_quote"},
						"properties": map[string]any{
							"id":           map[string]any{"type": "string"},
							"text":         map[string]any{"type": "string"},
							"role":         map[string]any{"type": "string"},
							"attaches_to":  map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "refine":
		return &llm.Schema{
			Name:     "compile_refine",
			Required: []string{"replacements"},
			Properties: map[string]any{
				"replacements": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"replace_id", "nodes", "reason"},
						"properties": map[string]any{
							"replace_id": map[string]any{"type": "string"},
							"reason":     map[string]any{"type": "string"},
							"nodes": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type":     "object",
									"required": []string{"text", "source_quote", "role"},
									"properties": map[string]any{
										"text":         map[string]any{"type": "string"},
										"source_quote": map[string]any{"type": "string"},
										"role":         map[string]any{"type": "string"},
									},
								},
							},
						},
					},
				},
			},
		}
	case "aggregate":
		return &llm.Schema{
			Name:     "compile_aggregate",
			Required: []string{"aggregates"},
			Properties: map[string]any{
				"aggregates": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"text", "member_ids", "source_quote", "reason"},
						"properties": map[string]any{
							"text": map[string]any{"type": "string"},
							"member_ids": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
							"source_quote": map[string]any{"type": "string"},
							"reason":       map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "support":
		return &llm.Schema{
			Name:     "compile_support",
			Required: []string{"support_edges"},
			Properties: map[string]any{
				"support_edges": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"from", "to", "kind", "source_quote", "reason"},
						"properties": map[string]any{
							"from":         map[string]any{"type": "string"},
							"to":           map[string]any{"type": "string"},
							"kind":         map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
							"reason":       map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "evidence":
		return linkListSchema("compile_evidence", "support_links", "from", "to")
	case "explanation":
		return linkListSchema("compile_explanation", "explanation_links", "from", "to")
	case "supplement":
		return linkListSchema("compile_supplement", "supplement_links", "a", "b")
	case "mainline":
		return mainlineSchema()
	case "validate":
		return &llm.Schema{
			Name:     "compile_validate",
			Required: []string{"missing_nodes", "missing_edges", "misclassified"},
			Properties: map[string]any{
				"missing_nodes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"text", "source_quote", "suggested_role_hint"},
						"properties": map[string]any{
							"text":                map[string]any{"type": "string"},
							"source_quote":        map[string]any{"type": "string"},
							"suggested_role_hint": map[string]any{"type": "string"},
						},
					},
				},
				"missing_edges": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"from_text", "to_text"},
						"properties": map[string]any{
							"from_text": map[string]any{"type": "string"},
							"to_text":   map[string]any{"type": "string"},
						},
					},
				},
				"misclassified": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"node_id", "issue"},
						"properties": map[string]any{
							"node_id": map[string]any{"type": "string"},
							"issue":   map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "translate":
		return &llm.Schema{
			Name:     "compile_translate",
			Required: []string{"translations"},
			Properties: map[string]any{
				"translations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text"},
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "summary":
		return &llm.Schema{
			Name:       "compile_summary",
			Required:   []string{"summary"},
			Properties: map[string]any{"summary": map[string]any{"type": "string"}},
		}
	default:
		return nil
	}
}

func mainlineSchema() *llm.Schema {
	schema := linkListSchema("compile_relations", "relations", "from", "to")
	if relations, ok := schema.Properties["relations"].(map[string]any); ok {
		if items, ok := relations["items"].(map[string]any); ok {
			if props, ok := items["properties"].(map[string]any); ok {
				props["kind"] = map[string]any{"type": "string"}
			}
		}
	}
	schema.Properties["spines"] = map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":     "object",
			"required": []string{"id", "level", "priority", "thesis", "node_ids", "edge_indexes", "scope", "why"},
			"properties": map[string]any{
				"id":           map[string]any{"type": "string"},
				"level":        map[string]any{"type": "string"},
				"priority":     map[string]any{"type": "integer"},
				"policy":       map[string]any{"type": "string"},
				"thesis":       map[string]any{"type": "string"},
				"node_ids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"edge_indexes": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
				"scope":        map[string]any{"type": "string"},
				"why":          map[string]any{"type": "string"},
			},
		},
	}
	return schema
}

func linkListSchema(name string, key string, fromKey string, toKey string) *llm.Schema {
	return &llm.Schema{
		Name:     name,
		Required: []string{key},
		Properties: map[string]any{
			key: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{fromKey, toKey, "source_quote", "reason"},
					"properties": map[string]any{
						fromKey:        map[string]any{"type": "string"},
						toKey:          map[string]any{"type": "string"},
						"source_quote": map[string]any{"type": "string"},
						"reason":       map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func stageModel(stageName, fallback string) string {
	switch strings.TrimSpace(stageName) {
	case "validate":
		return compile.Qwen36PlusModel
	default:
		if strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
		return compile.Qwen3MaxModel
	}
}

func stageSearch(stageName string) bool {
	switch strings.TrimSpace(stageName) {
	case "validate":
		return true
	default:
		return false
	}
}

func renderOffGraph(items []offGraphItem, cn func(id, fallback string) string) (evidence, explanation, supplementary []string) {
	for _, item := range items {
		switch item.Role {
		case "evidence", "inference":
			evidence = append(evidence, cn(item.ID, item.Text))
		case "explanation":
			explanation = append(explanation, cn(item.ID, item.Text))
		default:
			supplementary = append(supplementary, cn(item.ID, item.Text))
		}
	}
	return
}

func pruneDanglingEdges(state graphState) graphState {
	valid := map[string]struct{}{}
	for _, n := range state.Nodes {
		valid[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = dedupeEdges(edges)
	return state
}

func fallbackSummary(drivers, targets []string) string {
	switch {
	case len(drivers) > 0 && len(targets) > 0:
		return fmt.Sprintf("%s影响%s。", drivers[0], targets[0])
	case len(targets) > 0:
		return fmt.Sprintf("核心结果：%s。", targets[0])
	case len(drivers) > 0:
		return fmt.Sprintf("核心驱动：%s。", drivers[0])
	default:
		return "未能稳定提取主线。"
	}
}

func confidenceFromState(drivers, targets []string, paths []compile.TransmissionPath) string {
	if len(paths) > 0 && len(drivers) > 0 && len(targets) > 0 {
		return "medium"
	}
	if len(drivers) > 0 || len(targets) > 0 {
		return "low"
	}
	return "low"
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func serializeNodeList(nodes []graphNode) string {
	return joinSerializedLines(len(nodes), func(out *[]string) {
		for _, n := range nodes {
			*out = append(*out, fmt.Sprintf("%s: %s", n.ID, n.Text))
		}
	})
}

func serializeEdgeList(edges []graphEdge) string {
	return joinSerializedLines(len(edges), func(out *[]string) {
		for _, e := range edges {
			*out = append(*out, fmt.Sprintf("%s -> %s", e.From, e.To))
		}
	})
}

func serializeRelationNodes(nodes []graphNode) string {
	return joinSerializedLines(len(nodes), func(out *[]string) {
		for _, n := range nodes {
			*out = append(*out, fmt.Sprintf("%s | %s | role=%s | discourse_role=%s | ontology=%s | quote=%s", n.ID, n.Text, n.Role, n.DiscourseRole, n.Ontology, n.SourceQuote))
		}
	})
}

func serializeBranchHeads(state graphState) string {
	return joinSerializedLines(len(state.BranchHeads), func(out *[]string) {
		for _, id := range state.BranchHeads {
			node, ok := nodeByID(state.Nodes, id)
			if !ok {
				continue
			}
			*out = append(*out, fmt.Sprintf("%s | %s", node.ID, node.Text))
		}
	})
}

func serializeMainlineCandidateEdges(article string, nodes []graphNode) string {
	type candidate struct {
		from   graphNode
		to     graphNode
		quote  string
		reason string
	}
	candidates := make([]candidate, 0)
	for _, from := range nodes {
		for _, to := range nodes {
			if from.ID == to.ID {
				continue
			}
			if !eligibleForMainlineCandidateHint(from) || !eligibleForMainlineCandidateHint(to) {
				continue
			}
			if quote, reason, ok := suggestMainlineCandidate(article, from, to); ok {
				candidates = append(candidates, candidate{from: from, to: to, quote: quote, reason: reason})
			}
		}
	}
	if len(candidates) == 0 {
		return "- (none)"
	}
	var b strings.Builder
	for _, c := range candidates {
		fmt.Fprintf(&b, "- %s [%s] -> %s [%s] | quote=%s | hint=%s\n", c.from.ID, c.from.Text, c.to.ID, c.to.Text, c.quote, c.reason)
	}
	return strings.TrimSpace(b.String())
}

func eligibleForMainlineCandidateHint(node graphNode) bool {
	switch normalizeDiscourseRole(node.DiscourseRole) {
	case "evidence", "example", "caveat":
		return false
	default:
		return true
	}
}

func suggestMainlineCandidate(article string, from, to graphNode) (string, string, bool) {
	fromText := strings.TrimSpace(from.Text)
	toText := strings.TrimSpace(to.Text)
	fromQuote := strings.TrimSpace(from.SourceQuote)
	toQuote := strings.TrimSpace(to.SourceQuote)
	for _, quote := range mainlineCandidateQuotes(fromQuote, toQuote) {
		if !quoteDirectlyGroundsMainline(quote, fromText, toText) {
			continue
		}
		switch {
		case isRatePressureBridge(fromText, toText, quote):
			return quote, "rate-state bridge directly grounded by quote", true
		case isOilPriceBridge(fromText, toText, quote):
			return quote, "oil-price bridge directly grounded by quote", true
		default:
			return quote, "direct quote contains source, target, and drive wording", true
		}
	}
	if quote, reason, ok := suggestArticleWindowMainlineCandidate(article, from, to); ok {
		return quote, reason, true
	}
	return "", "", false
}

func suggestArticleWindowMainlineCandidate(article string, from, to graphNode) (string, string, bool) {
	fromText := strings.TrimSpace(from.Text)
	toText := strings.TrimSpace(to.Text)
	fromQuote := strings.TrimSpace(from.SourceQuote)
	toQuote := strings.TrimSpace(to.SourceQuote)
	quote := articleWindowForQuotes(article, fromQuote, toQuote)
	if strings.TrimSpace(quote) == "" {
		quote = combineQuoteWindow(fromQuote, toQuote)
	}
	if strings.TrimSpace(quote) == "" {
		return "", "", false
	}
	switch {
	case isFinancialClaimsCycleBridge(fromText, toText, quote):
		return quote, "article-window bridge for financial-claims cycle spine", true
	case isPetrodollarAssetPressureBridge(fromText, toText, quote):
		return quote, "article-window bridge from reduced US-asset buying to asset pressure", true
	case isCrowdedTradeOutflowBridge(fromText, toText, quote):
		return quote, "article-window bridge from crowded positioning to outflow volatility risk", true
	case isRedemptionRunGateBridge(fromText, toText, quote):
		return quote, "article-window bridge from redemption run to gate/panic mechanics", true
	case isPrivateCreditExposureDefaultBridge(fromText, toText, quote):
		return quote, "article-window bridge from private-credit exposure to default risk", true
	case isPrivateCreditRiskRedemptionBridge(fromText, toText, quote):
		return quote, "article-window bridge from private-credit risk to redemption pressure", true
	case isPrivateCreditWithdrawalRunBridge(fromText, toText, quote):
		return quote, "article-window bridge from withdrawn private-credit funding to redemption run", true
	case isPrivateCreditWithdrawalGateBridge(fromText, toText, quote):
		return quote, "article-window bridge from withdrawn private-credit funding to redemption gate", true
	default:
		return "", "", false
	}
}

func isFinancialClaimsCycleBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	markers := append(supportDriveMarkers(), "made", "allow", "allows", "allowed", "enable", "enables", "enabled", "became", "become", "后")
	if !containsAnyText(quoteLower, markers) {
		return false
	}
	fromFinancialPromise := containsAnyText(fromLower, []string{"金融财富", "承诺", "索取权", "financial wealth", "promise", "claim"})
	toMoneyUnconstrained := containsAnyText(toLower, []string{"不再受金银约束", "金银约束", "硬通货", "gold", "silver", "hard money"})
	if fromFinancialPromise && toMoneyUnconstrained && containsAnyText(quoteLower, []string{"金融财富", "financial wealth", "金银", "gold", "silver"}) {
		return true
	}
	fromMoneyUnconstrained := containsAnyText(fromLower, []string{"不再受金银约束", "金银约束", "硬通货", "gold", "silver", "hard money"})
	toFinancing := containsAnyText(toLower, []string{"借贷", "发行股票", "融资", "borrow", "borrowing", "stock", "finance", "financing", "credit"})
	if fromMoneyUnconstrained && toFinancing && containsAnyText(quoteLower, []string{"借贷", "发行股票", "融资", "borrow", "stock", "finance", "credit"}) {
		return true
	}
	fromFinancing := containsAnyText(fromLower, []string{"借贷", "发行股票", "融资", "borrow", "borrowing", "stock", "finance", "financing", "credit"})
	toFinancialWealthIncrease := containsAnyText(toLower, []string{"金融财富增加", "金融财富增长", "financial wealth increase", "financial wealth growth"})
	if fromFinancing && toFinancialWealthIncrease && containsAnyText(quoteLower, []string{"金融财富", "financial wealth"}) {
		return true
	}
	fromFinancialWealthIncrease := containsAnyText(fromLower, []string{"金融财富增加", "金融财富增长", "financial wealth increase", "financial wealth growth"})
	toPromiseCannotBeMet := containsAnyText(toLower, []string{"承诺无法兑现", "索取权", "义务", "有形财富", "无法兑现", "can't be met", "cannot be met", "claims", "obligations", "tangible wealth"})
	if fromFinancialWealthIncrease && toPromiseCannotBeMet && containsAnyText(quoteLower, []string{"承诺", "义务", "有形财富", "promise", "obligation", "tangible wealth", "can't be met", "cannot be met"}) {
		return true
	}
	fromPromiseCannotBeMet := containsAnyText(fromLower, []string{"承诺无法兑现", "索取权", "义务", "有形财富", "无法兑现", "can't be met", "cannot be met", "claims", "obligations", "tangible wealth"})
	toRealWealthDecline := containsAnyText(toLower, []string{"金融财富相对于真实财富下降", "真实财富", "real wealth", "devaluation", "贬值"})
	return fromPromiseCannotBeMet && toRealWealthDecline && containsAnyText(quoteLower, []string{"真实财富", "real wealth", "贬值", "devaluation", "印钞", "printing"})
}

func isPetrodollarAssetPressureBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"购买美债美股的资金减少", "买美债美股", "美债美股"})
	toOK := containsAnyText(toLower, []string{"美股", "美债", "美国的美元资产"}) && containsAnyText(toLower, []string{"压力", "流出", "承压"})
	quoteOK := containsAnyText(quoteLower, []string{"没这么多钱", "资金减少", "钱去买美债美股", "离开了美国", "美股 美债都会受到压力", "美股美债都会受到压力"})
	return fromOK && toOK && quoteOK
}

func isCrowdedTradeOutflowBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"拥挤", "crowded"}) && containsAnyText(fromLower, []string{"ai", "m7", "交易", "trade"})
	toOK := containsAnyText(toLower, []string{"资金净流出", "资产价格", "波动风险", "剧烈波动", "outflow"})
	quoteOK := containsAnyText(quoteLower, []string{"拥挤交易", "钱往外走", "没钱往里进", "随时可能出事", "资产价格的变化"})
	return fromOK && toOK && quoteOK
}

func isRedemptionRunGateBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"挤兑", "集中赎回", "redemption run"})
	toOK := containsAnyText(toLower, []string{"赎回"}) && containsAnyText(toLower, []string{"上限", "限制", "额度", "关门", "gate"})
	quoteOK := containsAnyText(quoteLower, []string{"赎回", "额度", "关门", "最多只能赎回", "下个季度"})
	return fromOK && toOK && quoteOK
}

func isPrivateCreditExposureDefaultBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"私募信贷资金", "私募信贷融资", "长期数据中心租约", "资金大量流入", "偿还私募信贷贷款"})
	toOK := containsAnyText(toLower, []string{"违约风险", "资产安全", "偿还", "贷款"}) && containsAnyText(toLower, []string{"私募信贷", "项目", "贷款"})
	quoteOK := containsAnyText(quoteLower, []string{"私募信贷", "借给", "数据中心", "贷过去的钱", "完蛋", "违约", "换账", "破产"})
	return fromOK && toOK && quoteOK
}

func isPrivateCreditRiskRedemptionBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"违约风险", "面临违约", "贷款违约", "资产安全", "偿还", "盈利模式受损", "支付能力下降"})
	toOK := containsAnyText(toLower, []string{"集中赎回", "申请赎回", "赎回请求", "高净值客户", "私募信贷基金"})
	quoteOK := containsAnyText(quoteLower, []string{"可能就黄了", "可能换账", "完蛋", "开始追", "能不能赎回", "开始被挤提", "开始被几题"})
	return fromOK && toOK && quoteOK
}

func isPrivateCreditWithdrawalRunBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"中东资金", "中东投资者", "中东主权资金", "撤回", "撤出", "停止追加", "拿回来"})
	toOK := containsAnyText(toLower, []string{"集中赎回", "挤兑", "击提", "赎回压力", "赎回请求"})
	quoteOK := containsAnyText(quoteLower, []string{"不会再往里贴钱", "开始把钱拿回来", "遭到击提", "遭到挤提", "赎回", "开始追"})
	return fromOK && toOK && quoteOK
}

func isPrivateCreditWithdrawalGateBridge(fromText, toText, quote string) bool {
	fromLower := strings.ToLower(strings.TrimSpace(fromText))
	toLower := strings.ToLower(strings.TrimSpace(toText))
	quoteLower := strings.ToLower(strings.TrimSpace(quote))
	fromOK := containsAnyText(fromLower, []string{"中东资金", "中东主权资金", "撤出", "停止追加", "拿回来"})
	toOK := containsAnyText(toLower, []string{"赎回上限", "暂停当期赎回", "暂停赎回", "赎回额度", "关门"})
	quoteOK := containsAnyText(quoteLower, []string{"不会再往里贴钱", "开始把钱拿回来", "遭到击提", "赎回", "额度", "关门", "下个季度"})
	return fromOK && toOK && quoteOK
}

func articleWindowForQuotes(article, fromQuote, toQuote string) string {
	article = strings.TrimSpace(article)
	if article == "" {
		return ""
	}
	fromRange, okFrom := findQuoteRange(article, fromQuote)
	toRange, okTo := findQuoteRange(article, toQuote)
	if !okFrom || !okTo {
		return ""
	}
	start := fromRange.start
	end := toRange.end
	if toRange.start < fromRange.start {
		start = toRange.start
		end = fromRange.end
	}
	if start < 0 || end <= start || end > len(article) {
		return ""
	}
	window := article[start:end]
	const maxWindowRunes = 900
	if len([]rune(window)) > maxWindowRunes {
		return combineQuoteWindow(fromQuote, toQuote)
	}
	return strings.TrimSpace(window)
}

type textRange struct {
	start int
	end   int
}

func findQuoteRange(text, quote string) (textRange, bool) {
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return textRange{}, false
	}
	if idx := strings.Index(text, quote); idx >= 0 {
		return textRange{start: idx, end: idx + len(quote)}, true
	}
	pieces := meaningfulQuotePieces(quote)
	if len(pieces) == 0 {
		return textRange{}, false
	}
	first := pieces[0]
	start := strings.Index(text, first)
	if start < 0 {
		return textRange{}, false
	}
	end := start + len(first)
	searchFrom := end
	for _, piece := range pieces[1:] {
		next := strings.Index(text[searchFrom:], piece)
		if next < 0 {
			continue
		}
		end = searchFrom + next + len(piece)
		searchFrom = end
	}
	return textRange{start: start, end: end}, true
}

func meaningfulQuotePieces(quote string) []string {
	raw := strings.Split(quote, "...")
	pieces := make([]string, 0, len(raw))
	for _, piece := range raw {
		piece = strings.TrimSpace(piece)
		if len([]rune(piece)) < 4 {
			continue
		}
		pieces = append(pieces, piece)
	}
	return pieces
}

func combineQuoteWindow(fromQuote, toQuote string) string {
	fromQuote = strings.TrimSpace(fromQuote)
	toQuote = strings.TrimSpace(toQuote)
	switch {
	case fromQuote == "":
		return toQuote
	case toQuote == "":
		return fromQuote
	case strings.EqualFold(fromQuote, toQuote):
		return fromQuote
	default:
		return fromQuote + " ... " + toQuote
	}
}

func isRatePressureBridge(fromText, toText, toQuote string) bool {
	fromLower := strings.ToLower(fromText)
	toLower := strings.ToLower(toText + " " + toQuote)
	if !containsAnyText(fromLower, []string{"利率维持高位", "利率上升", "高利率"}) {
		return false
	}
	return containsAnyText(toLower, []string{"资产价格", "所有资产", "融资成本", "房贷成本", "企业融资", "长期债券", "价格承压", "下行压力"})
}

func isOilPriceBridge(fromText, toText, toQuote string) bool {
	fromLower := strings.ToLower(fromText)
	toLower := strings.ToLower(toText + " " + toQuote)
	if !containsAnyText(fromLower, []string{"油价上涨", "原油价格", "布伦特原油"}) {
		return false
	}
	return containsAnyText(toLower, []string{"下游成本", "成本上升", "消费品价格", "通胀"})
}

func mainlineCandidateQuotes(fromQuote, toQuote string) []string {
	quotes := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, quote := range []string{fromQuote, toQuote} {
		quote = strings.TrimSpace(quote)
		if quote == "" {
			continue
		}
		key := strings.ToLower(quote)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		quotes = append(quotes, quote)
	}
	return quotes
}

func quoteDirectlyGroundsMainline(quote, fromText, toText string) bool {
	fromAnchors := endpointAnchors(fromText)
	toAnchors := endpointAnchors(toText)
	if len(fromAnchors) == 0 || len(toAnchors) == 0 {
		return false
	}
	for _, clause := range splitQuoteClauses(quote) {
		clauseLower := strings.ToLower(strings.TrimSpace(clause))
		if !containsAnyText(clauseLower, supportDriveMarkers()) {
			continue
		}
		fromPos := firstAnchorPosition(clauseLower, fromAnchors)
		toPos := firstAnchorPosition(clauseLower, toAnchors)
		if fromPos >= 0 && toPos >= 0 && fromPos <= toPos {
			return true
		}
	}
	return false
}

func splitQuoteClauses(quote string) []string {
	clauses := strings.FieldsFunc(quote, func(r rune) bool {
		switch r {
		case ';', '；', '。', '!', '！', '?', '？', '\n', '\r':
			return true
		default:
			return false
		}
	})
	if len(clauses) == 0 {
		return []string{quote}
	}
	return clauses
}

func firstAnchorPosition(text string, anchors []string) int {
	best := -1
	for _, anchor := range anchors {
		anchor = strings.ToLower(strings.TrimSpace(anchor))
		if anchor == "" {
			continue
		}
		pos := strings.Index(text, anchor)
		if pos < 0 {
			continue
		}
		if best < 0 || pos < best {
			best = pos
		}
	}
	return best
}

func endpointAnchors(text string) []string {
	lower := strings.ToLower(strings.TrimSpace(text))
	anchors := make([]string, 0, 8)
	switch {
	case containsAnyText(lower, []string{"资产价格", "所有资产", "股票价格", "债券价格", "房产价格", "私募资产价格"}):
		anchors = append(anchors, "所有资产价格", "资产价格", "所有资产", "股票", "债券", "房产", "私募", "下行压力", "压低", "承压")
	case containsAnyText(lower, []string{"利率维持高位", "利率上升", "高利率"}):
		anchors = append(anchors, "高利率", "利率")
	case containsAnyText(lower, []string{"油价上涨", "原油价格", "布伦特原油"}):
		anchors = append(anchors, "油价", "原油", "布伦特原油")
	case containsAnyText(lower, []string{"下游成本", "成本上升", "消费品价格", "通胀"}):
		anchors = append(anchors, "下游成本", "成本上升", "消费品价格", "通胀")
	case containsAnyText(lower, []string{"赎回请求", "赎回申请", "赎回"}):
		anchors = append(anchors, "赎回请求", "赎回申请", "赎回", "redemption")
	case containsAnyText(lower, []string{"流动性资产", "流动性压力", "行业流动性"}):
		anchors = append(anchors, "流动性资产", "流动性压力", "行业流动性", "流动性")
	case containsAnyText(lower, []string{"现金", "cash"}):
		anchors = append(anchors, "现金", "cash")
	}
	anchors = append(anchors, genericEndpointAnchors(lower)...)
	return dedupeStrings(anchors)
}

func genericEndpointAnchors(text string) []string {
	anchors := make([]string, 0, 4)
	for _, marker := range []string{
		"财政刺激", "财政", "债务", "利息", "国防预算", "云资本开支", "资产", "成本", "通胀",
		"inflation", "debt", "interest", "fiscal", "cloud capex", "liquidity", "redemption",
	} {
		if strings.Contains(text, strings.ToLower(marker)) {
			anchors = append(anchors, marker)
		}
	}
	if len([]rune(text)) <= 32 {
		anchors = append(anchors, text)
	}
	return anchors
}

func joinSerializedLines(capacity int, appendLines func(*[]string)) string {
	lines := make([]string, 0, capacity)
	appendLines(&lines)
	return strings.Join(lines, "\n")
}

func isOutcomeLikeNode(n graphNode) bool {
	if n.IsTarget {
		return true
	}
	if strings.TrimSpace(n.Ontology) != "" && strings.TrimSpace(n.Ontology) != "none" {
		return true
	}
	return false
}

func inferTargetKind(text string, isTarget bool) string {
	if !isTarget {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{"利率", "yield", "rate", "息差"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return "rate"
		}
	}
	for _, marker := range []string{"资金", "流入", "流出", "赎回", "流动性", "liquidity", "flow", "资金被锁定", "allocation"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return "flow"
		}
	}
	for _, marker := range []string{"价格", "price", "油价", "原油", "股价", "债券", "上涨", "下跌"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return "price"
		}
	}
	for _, marker := range []string{"政策", "decision", "批准", "加息", "降息"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return "decision"
		}
	}
	return "none"
}

func shouldDemoteSupportToSupplement(fromNode, toNode graphNode) bool {
	return isOutcomeLikeNode(fromNode) && isOutcomeLikeNode(toNode)
}

func chooseSupplementPrimary(left, right graphNode) (string, graphNode) {
	if isLabelLikeNode(left.Text) && !isLabelLikeNode(right.Text) {
		return right.ID, left
	}
	if isLabelLikeNode(right.Text) && !isLabelLikeNode(left.Text) {
		return left.ID, right
	}
	if directnessScore(left.Text) >= directnessScore(right.Text) {
		return left.ID, right
	}
	return right.ID, left
}

func isLabelLikeNode(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{" trade", " narrative", "交易", "叙事", "story", "regime"} {
		if strings.Contains(t, marker) {
			return true
		}
	}
	return false
}

func directnessScore(text string) int {
	score := 0
	t := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{"流入", "流出", "上涨", "下跌", "rise", "fall", "inflow", "outflow", "yield", "spread", "price", "position", "hedge", "allocation"} {
		if strings.Contains(t, strings.ToLower(marker)) {
			score += 2
		}
	}
	if !isLabelLikeNode(text) {
		score++
	}
	if len([]rune(text)) < 40 {
		score++
	}
	return score
}

func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
