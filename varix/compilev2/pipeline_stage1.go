package compilev2

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

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
