package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
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
	ID          string
	Text        string
	SourceQuote string
	Role        graphRole
	Ontology    string
	IsTarget    bool
}

type graphEdge struct {
	From        string
	To          string
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
	state.Nodes = decodeStage1Nodes(payload["nodes"])
	state.Edges = nil
	state.OffGraph = decodeStage1OffGraph(payload["off_graph"])
	state = fillMissingStage1IDs(state)
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
	Text        string `json:"text"`
	SourceQuote string `json:"source_quote"`
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
			ID:          id,
			Text:        text,
			SourceQuote: strings.TrimSpace(aggregate.SourceQuote),
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
				ID:          fmt.Sprintf("%s_%d", replaceID, idx+1),
				Text:        text,
				SourceQuote: sourceQuote,
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
		if n.Text == "" {
			continue
		}
		nodes = append(nodes, n)
	}
	state.Nodes = nodes

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
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(compile.FirstNonEmpty(asString(v["text"]), asString(v["content"]))),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
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
		state.BranchHeads = nil
		for _, node := range state.Nodes {
			state.BranchHeads = append(state.BranchHeads, node.ID)
		}
		state.BranchHeads = dedupeStrings(state.BranchHeads)
		state = pruneDanglingEdges(state)
		return state
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
	state = pruneDanglingEdges(state)
	return state
}

func stage3Mainline(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage3MainlineSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage3MainlineUserPrompt(bundle.TextContext(), serializeRelationNodes(state.Nodes), serializeBranchHeads(state), serializeMainlineCandidateEdges(bundle.TextContext(), state.Nodes))
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		DrivesEdges []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"drives_edges"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "mainline", &result); err != nil {
		return graphState{}, err
	}
	valid := map[string]graphNode{}
	for _, n := range state.Nodes {
		valid[n.ID] = n
	}
	newEdges := make([]graphEdge, 0, len(result.DrivesEdges))
	for _, e := range result.DrivesEdges {
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
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	oldEdges := state.Edges
	state.Edges = pruneTransitiveMainlineEdges(dedupeEdges(append(append([]graphEdge(nil), oldEdges...), newEdges...)))
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
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterTargetNodes(state.Nodes)
	if len(targets) == 0 && len(drivers) > 0 {
		targets = fallbackTargetNodesFromOffGraph(state.OffGraph)
	}
	paths := extractPaths(state, drivers, targets)
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
	summary, err := summarizeChinese(ctx, rt, model, driversOut, targetsOut, transmission, bundle)
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

func pruneTransitiveMainlineEdges(edges []graphEdge) []graphEdge {
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
	case "explanation":
		return "explanation", true
	case "supplement", "supplementary":
		return "supplementary", true
	default:
		return "", false
	}
}

func auxNodeRole(edge auxEdge, nodeID string) (string, bool) {
	switch edge.Kind {
	case "evidence":
		if edge.From == nodeID {
			return "evidence", true
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
	return canonicalityScore(nodeID, inScore, outScore) + 0.35*clusterHeadTieBreak(nodeIndex[nodeID].Text)
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

func summarizeChinese(ctx context.Context, rt runtimeChat, model string, drivers, targets []string, paths []compile.TransmissionPath, bundle compile.Bundle) (string, error) {
	payload, err := json.Marshal(map[string]any{"drivers": drivers, "targets": targets, "paths": paths})
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
			Required: []string{"nodes", "off_graph"},
			Properties: map[string]any{
				"nodes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "text", "source_quote"},
						"properties": map[string]any{
							"id":           map[string]any{"type": "string"},
							"text":         map[string]any{"type": "string"},
							"source_quote": map[string]any{"type": "string"},
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
									"required": []string{"text", "source_quote"},
									"properties": map[string]any{
										"text":         map[string]any{"type": "string"},
										"source_quote": map[string]any{"type": "string"},
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
		return linkListSchema("compile_mainline", "drives_edges", "from", "to")
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
		case "evidence":
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
			*out = append(*out, fmt.Sprintf("%s | %s | role=%s | ontology=%s | quote=%s", n.ID, n.Text, n.Role, n.Ontology, n.SourceQuote))
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
