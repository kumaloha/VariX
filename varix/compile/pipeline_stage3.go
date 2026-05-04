package compile

import (
	"context"
	"fmt"
	"strings"
)

func stage3Classify(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
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
		if isDeclarationSpinePolicy(spine.Policy) {
			continue
		}
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
	UnitIDs     []string `json:"unit_ids"`
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
	case "management_qa", "shareholder_meeting", "earnings_call", "capital_allocation_discussion":
		return spinePolicy{
			ArticleForm:                 form,
			PrimaryMode:                 "required",
			MinSpines:                   1,
			MaxSpines:                   4,
			MaxLocal:                    1,
			MergeSameFamilyBranches:     true,
			AllowSingleNodeFamilySpines: true,
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
	case "management_qa", "shareholder_meeting", "earnings_call", "capital_allocation_discussion":
		return "management_qa/shareholder_meeting/earnings_call: prioritize management declarations over incidental business causal chains. Use policy=capital_allocation_rule when management states how cash/capital will be deployed, including speaker, condition, action, scale, constraint, and non-action nodes. Use policy=management_declaration for other explicit management commitments, guidance, operating plans, or boundaries. Do not force a management declaration into a driver -> target causal chain; it may be a declaration spine with one central statement plus supporting condition/action/evidence nodes."
	default:
		return "single_thesis/default: keep the shortest sufficient primary causal spine, with only major derived branch spines."
	}
}

func stage3Mainline(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage3MainlineSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage3MainlineUserPrompt(bundle.TextContext(), state.ArticleForm, serializeRelationNodes(state.Nodes), serializeBranchHeads(state), serializeSemanticUnitsForMainline(state.SemanticUnits), serializeMainlineCandidateEdges(bundle.TextContext(), state.Nodes, state.CoverageHints))
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
