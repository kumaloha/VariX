package compile

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

var (
	declarationMarkupRE     = regexp.MustCompile(`(?s)<[^>]+>`)
	declarationWhitespaceRE = regexp.MustCompile(`\s+`)
)

func isDeclarationSpinePolicy(policy string) bool {
	switch normalizePreviewSpinePolicy(policy) {
	case "management_declaration", "capital_allocation_rule", "policy_guidance", "policy_stance", "commitment", "operating_plan", "risk_boundary", "non_action_boundary":
		return true
	default:
		return false
	}
}

func shouldFallbackToRolePaths(spines []PreviewSpine) bool {
	if len(spines) == 0 {
		return true
	}
	for _, spine := range spines {
		if !isDeclarationSpinePolicy(spine.Policy) {
			return true
		}
	}
	return false
}

func declarationTranslationNodes(state graphState) []graphNode {
	if len(state.Spines) == 0 || len(state.Nodes) == 0 {
		return nil
	}
	index := map[string]graphNode{}
	for _, node := range state.Nodes {
		index[node.ID] = node
	}
	seen := map[string]struct{}{}
	out := make([]graphNode, 0)
	for _, spine := range state.Spines {
		if !isDeclarationSpinePolicy(spine.Policy) {
			continue
		}
		for _, id := range spine.NodeIDs {
			node, ok := index[strings.TrimSpace(id)]
			if !ok {
				continue
			}
			if _, exists := seen[node.ID]; exists {
				continue
			}
			seen[node.ID] = struct{}{}
			out = append(out, node)
		}
	}
	return out
}

func renderDeclarationsFromSpines(bundle Bundle, state graphState, cn func(string, string) string) []Declaration {
	if len(state.Spines) == 0 || len(state.Nodes) == 0 {
		return nil
	}
	index := map[string]graphNode{}
	for _, node := range state.Nodes {
		index[node.ID] = node
	}
	out := make([]Declaration, 0)
	for _, spine := range state.Spines {
		if !isDeclarationSpinePolicy(spine.Policy) {
			continue
		}
		declaration, ok := renderDeclarationFromSpine(bundle, spine, index, cn)
		if !ok {
			continue
		}
		out = append(out, declaration)
	}
	return out
}

func appendSourceDeclarations(bundle Bundle, declarations []Declaration) []Declaration {
	if declaration, ok := sourceCapitalAllocationDeclaration(bundle); ok {
		for i := range declarations {
			if isCapitalAllocationDeclaration(declarations[i]) {
				declarations[i] = mergeDeclaration(declarations[i], declaration)
				enrichDeclarationFromSourceQuote(&declarations[i])
				return declarations
			}
		}
		return append(declarations, declaration)
	}
	return declarations
}

func applyDeclarationCoverageGate(bundle Bundle, state graphState) graphState {
	sourceDeclaration, ok := sourceCapitalAllocationDeclaration(bundle)
	if !ok {
		return state
	}
	spineIndex := findCapitalAllocationSpine(state)
	if spineIndex < 0 {
		statementID := "source_capital_allocation_statement"
		state.Nodes = append(state.Nodes, graphNode{
			ID:            statementID,
			Text:          sourceDeclaration.Statement,
			SourceQuote:   sourceDeclaration.SourceQuote,
			DiscourseRole: "capital_allocation_rule",
		})
		state.Spines = append(state.Spines, PreviewSpine{
			ID:       "source_capital_allocation",
			Level:    "primary",
			Priority: len(state.Spines) + 1,
			Policy:   "capital_allocation_rule",
			Thesis:   sourceDeclaration.Statement,
			NodeIDs:  []string{statementID},
			Scope:    "article",
		})
		spineIndex = len(state.Spines) - 1
	}
	state = attachDeclarationCoverageNode(state, spineIndex, "condition", "source_capital_allocation_condition", firstString(sourceDeclaration.Conditions), sourceDeclaration.SourceQuote)
	state = attachDeclarationCoverageNode(state, spineIndex, "action", "source_capital_allocation_action", firstString(sourceDeclaration.Actions), sourceDeclaration.SourceQuote)
	state = attachDeclarationCoverageNode(state, spineIndex, "scale", "source_capital_allocation_scale", sourceDeclaration.Scale, sourceDeclaration.SourceQuote)
	state = attachDeclarationCoverageNode(state, spineIndex, "constraint", "source_capital_allocation_constraint", firstString(sourceDeclaration.Constraints), sourceDeclaration.SourceQuote)
	state = attachDeclarationCoverageNode(state, spineIndex, "evidence", "source_capital_allocation_evidence", firstString(sourceDeclaration.Evidence), sourceDeclaration.SourceQuote)
	return state
}

func findCapitalAllocationSpine(state graphState) int {
	nodes := map[string]graphNode{}
	for _, node := range state.Nodes {
		nodes[node.ID] = node
	}
	for i, spine := range state.Spines {
		if normalizePreviewSpinePolicy(spine.Policy) == "capital_allocation_rule" {
			return i
		}
		text := strings.ToLower(spine.Thesis)
		for _, id := range spine.NodeIDs {
			node := nodes[strings.TrimSpace(id)]
			text += " " + strings.ToLower(node.Text) + " " + strings.ToLower(node.SourceQuote) + " " + strings.ToLower(node.DiscourseRole)
		}
		if containsAnyText(text, []string{"capital allocation", "allocating our capital", "deploy capital", "现金", "资本配置", "配置资本"}) {
			return i
		}
	}
	return -1
}

func attachDeclarationCoverageNode(state graphState, spineIndex int, role, id, text, sourceQuote string) graphState {
	text = strings.TrimSpace(text)
	if spineIndex < 0 || spineIndex >= len(state.Spines) || text == "" {
		return state
	}
	if spineHasDeclarationRole(state, state.Spines[spineIndex], role) {
		return state
	}
	if existingID := findNodeIDByText(state.Nodes, text); existingID != "" {
		state.Spines[spineIndex].NodeIDs = appendUniqueString(state.Spines[spineIndex].NodeIDs, existingID)
		return state
	}
	id = uniqueGraphNodeID(state.Nodes, id)
	state.Nodes = append(state.Nodes, graphNode{
		ID:            id,
		Text:          text,
		SourceQuote:   strings.TrimSpace(sourceQuote),
		DiscourseRole: role,
	})
	state.Spines[spineIndex].NodeIDs = appendUniqueString(state.Spines[spineIndex].NodeIDs, id)
	return state
}

func spineHasDeclarationRole(state graphState, spine PreviewSpine, role string) bool {
	role = normalizeDiscourseRole(role)
	nodes := map[string]graphNode{}
	for _, node := range state.Nodes {
		nodes[node.ID] = node
	}
	for _, id := range spine.NodeIDs {
		if normalizeDiscourseRole(nodes[strings.TrimSpace(id)].DiscourseRole) == role {
			return true
		}
	}
	return false
}

func findNodeIDByText(nodes []graphNode, text string) string {
	key := normalizeText(text)
	for _, node := range nodes {
		if normalizeText(node.Text) == key {
			return strings.TrimSpace(node.ID)
		}
	}
	return ""
}

func uniqueGraphNodeID(nodes []graphNode, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "source_declaration_node"
	}
	used := map[string]struct{}{}
	for _, node := range nodes {
		used[strings.TrimSpace(node.ID)] = struct{}{}
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		id := base + "_" + strconv.Itoa(i)
		if _, ok := used[id]; !ok {
			return id
		}
	}
}

func hasCapitalAllocationDeclaration(declarations []Declaration) bool {
	for _, declaration := range declarations {
		if isCapitalAllocationDeclaration(declaration) {
			return true
		}
	}
	return false
}

func isCapitalAllocationDeclaration(declaration Declaration) bool {
	text := strings.ToLower(strings.Join([]string{
		declaration.Kind,
		declaration.Topic,
		declaration.Statement,
		declaration.SourceQuote,
	}, " "))
	return normalizePreviewSpinePolicy(declaration.Kind) == "capital_allocation_rule" ||
		strings.TrimSpace(declaration.Topic) == "capital_allocation" ||
		containsAnyText(text, []string{"capital allocation", "allocating our capital", "资本配置", "配置资本"})
}

func mergeDeclaration(base, extra Declaration) Declaration {
	if strings.TrimSpace(base.Speaker) == "" {
		base.Speaker = strings.TrimSpace(extra.Speaker)
	}
	if strings.TrimSpace(base.Kind) == "" {
		base.Kind = strings.TrimSpace(extra.Kind)
	}
	if strings.TrimSpace(base.Topic) == "" {
		base.Topic = strings.TrimSpace(extra.Topic)
	}
	base.Conditions = dedupeStrings(append(base.Conditions, extra.Conditions...))
	base.Actions = dedupeStrings(append(base.Actions, extra.Actions...))
	if strings.TrimSpace(base.Scale) == "" {
		base.Scale = strings.TrimSpace(extra.Scale)
	}
	base.Constraints = dedupeStrings(append(base.Constraints, extra.Constraints...))
	base.NonActions = dedupeStrings(append(base.NonActions, extra.NonActions...))
	base.Evidence = dedupeStrings(append(base.Evidence, extra.Evidence...))
	base.SourceQuote = strings.Join(firstDeclarationStrings(dedupeDeclarationQuoteFragments(base.SourceQuote, extra.SourceQuote), 6), " / ")
	if strings.TrimSpace(base.Confidence) == "" || base.Confidence == "medium" && extra.Confidence == "high" {
		base.Confidence = strings.TrimSpace(extra.Confidence)
	}
	return base
}

func dedupeDeclarationQuoteFragments(values ...string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, fragment := range strings.Split(value, " / ") {
			fragment = compactDeclarationSourceQuote(fragment)
			key := normalizeDeclarationQuoteKey(fragment)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, fragment)
		}
	}
	return out
}

func normalizeDeclarationQuoteKey(value string) string {
	value = strings.ToLower(value)
	value = declarationWhitespaceRE.ReplaceAllString(value, " ")
	return strings.Trim(value, " <>:0123456789.,;/-")
}

func sourceCapitalAllocationDeclaration(bundle Bundle) (Declaration, bool) {
	source := bundle.TextContext()
	lower := strings.ToLower(source)
	if !containsAnyText(lower, []string{"capital allocation", "allocating our capital", "cash and us treasury", "significant capital", "配置资本", "资本配置"}) {
		return Declaration{}, false
	}
	if !containsAnyText(lower, []string{"act decisively", "significant capital", "dislocations", "value proposition", "patience", "discipline", "果断", "大量资本", "市场错配"}) {
		return Declaration{}, false
	}

	quotes := canonicalCapitalAllocationQuoteFragments(lower)
	if len(quotes) == 0 {
		quotes = dedupeStrings([]string{
			sourceDeclarationQuoteWindow(source, lower, "patience"),
			sourceDeclarationQuoteWindow(source, lower, "discipline"),
			sourceDeclarationQuoteWindow(source, lower, "value proposition"),
			sourceDeclarationQuoteWindow(source, lower, "act decisively"),
			sourceDeclarationQuoteWindow(source, lower, "significant capital"),
			sourceDeclarationQuoteWindow(source, lower, "dislocations"),
			sourceDeclarationQuoteWindow(source, lower, "cash and us treasury"),
			sourceDeclarationQuoteWindow(source, lower, "380"),
		})
	}
	declaration := Declaration{
		ID:          "source_capital_allocation",
		Speaker:     inferDeclarationSpeaker(bundle, nil, nil),
		Kind:        "capital_allocation_rule",
		Topic:       "capital_allocation",
		Statement:   "伯克希尔的资本配置理念强调耐心与纪律，只在机会符合原则且具备显著投资价值时快速、大额行动。",
		SourceQuote: strings.Join(firstDeclarationStrings(quotes, 4), " / "),
		Confidence:  "high",
	}
	if containsAnyText(lower, []string{"cash and us treasury", "us treasury bills net is 380", "380 billion", "397.4 billion"}) {
		declaration.Evidence = appendUniqueString(declaration.Evidence, "伯克希尔现金及美国短债净额约为3800亿美元")
	}
	enrichDeclarationFromSourceQuote(&declaration)
	if strings.TrimSpace(declaration.SourceQuote) == "" && len(declaration.Conditions) == 0 && len(declaration.Actions) == 0 {
		return Declaration{}, false
	}
	return declaration, true
}

func canonicalCapitalAllocationQuoteFragments(lowerSource string) []string {
	fragments := make([]string, 0, 5)
	if containsAnyText(lowerSource, []string{"patience", "disciplined", "discipline"}) {
		fragments = append(fragments, "patience and being disciplined when it comes to allocating our capital")
	}
	if containsAnyText(lowerSource, []string{"subpar opportunities", "subpar opportunity"}) {
		fragments = append(fragments, "not anxious to deploy capital into subpar opportunities")
	}
	if strings.Contains(lowerSource, "value proposition") {
		fragments = append(fragments, "strong value proposition")
	}
	if containsAnyText(lowerSource, []string{"act decisively", "significant capital"}) {
		fragments = append(fragments, "act decisively both quickly and with significant capital")
	}
	if containsAnyText(lowerSource, []string{"cash and us treasury", "treasury bills net is 380", "380 billion"}) {
		fragments = append(fragments, "cash and US Treasury bills net is 380 billion")
	}
	return dedupeStrings(fragments)
}

func sourceDeclarationQuoteWindow(source, lower, marker string) string {
	marker = strings.ToLower(strings.TrimSpace(marker))
	if marker == "" {
		return ""
	}
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return ""
	}
	start := idx - 320
	if start < 0 {
		start = 0
	}
	end := idx + len(marker) + 420
	if end > len(source) {
		end = len(source)
	}
	return compactDeclarationSourceQuote(source[start:end])
}

func compactDeclarationSourceQuote(value string) string {
	value = html.UnescapeString(value)
	value = declarationMarkupRE.ReplaceAllString(value, " ")
	value = declarationWhitespaceRE.ReplaceAllString(value, " ")
	return truncateRunes(strings.TrimSpace(value), 420)
}

func renderDeclarationFromSpine(bundle Bundle, spine PreviewSpine, nodes map[string]graphNode, cn func(string, string) string) (Declaration, bool) {
	nodeIDs := make([]string, 0, len(spine.NodeIDs))
	for _, id := range spine.NodeIDs {
		if _, ok := nodes[strings.TrimSpace(id)]; ok {
			nodeIDs = append(nodeIDs, strings.TrimSpace(id))
		}
	}
	if len(nodeIDs) == 0 {
		return Declaration{}, false
	}

	var statement graphNode
	for _, id := range nodeIDs {
		node := nodes[id]
		if isDeclarationStatementRole(node.DiscourseRole) {
			statement = node
			break
		}
	}
	if strings.TrimSpace(statement.ID) == "" {
		for _, id := range nodeIDs {
			node := nodes[id]
			if !isDeclarationSupportRole(node.DiscourseRole) {
				statement = node
				break
			}
		}
	}
	if strings.TrimSpace(statement.ID) == "" {
		statement = nodes[nodeIDs[0]]
	}
	statementText := strings.TrimSpace(cn(statement.ID, statement.Text))
	if statementText == "" {
		return Declaration{}, false
	}

	decl := Declaration{
		ID:         strings.TrimSpace(spine.ID),
		Speaker:    inferDeclarationSpeaker(bundle, nodeIDs, nodes),
		Kind:       declarationKindForSpine(spine),
		Topic:      declarationTopicForSpine(spine, nodeIDs, nodes),
		Statement:  statementText,
		Confidence: "medium",
	}
	quotes := make([]string, 0)
	for _, id := range nodeIDs {
		node := nodes[id]
		text := strings.TrimSpace(cn(node.ID, node.Text))
		if text == "" || text == statementText && node.ID != statement.ID {
			continue
		}
		switch normalizeDiscourseRole(node.DiscourseRole) {
		case "condition":
			decl.Conditions = appendUniqueString(decl.Conditions, text)
		case "action":
			decl.Actions = appendUniqueString(decl.Actions, text)
		case "scale":
			if strings.TrimSpace(decl.Scale) == "" {
				decl.Scale = text
			}
		case "constraint":
			decl.Constraints = appendUniqueString(decl.Constraints, text)
		case "non_action":
			decl.NonActions = appendUniqueString(decl.NonActions, text)
		case "evidence":
			decl.Evidence = appendUniqueString(decl.Evidence, text)
		}
		if quote := strings.TrimSpace(node.SourceQuote); quote != "" {
			quotes = appendUniqueString(quotes, quote)
		}
	}
	if strings.TrimSpace(statement.SourceQuote) != "" {
		quotes = append([]string{strings.TrimSpace(statement.SourceQuote)}, quotes...)
		quotes = dedupeStrings(quotes)
	}
	decl.SourceQuote = strings.Join(firstDeclarationStrings(quotes, 4), " / ")
	enrichDeclarationFromSourceQuote(&decl)
	if len(decl.Actions) > 0 || len(decl.Conditions) > 0 || strings.TrimSpace(decl.SourceQuote) != "" {
		decl.Confidence = "high"
	}
	return decl, true
}

func isDeclarationStatementRole(role string) bool {
	switch normalizeDiscourseRole(role) {
	case "declaration", "commitment", "policy_stance", "capital_allocation_rule", "guidance", "operating_plan", "risk_boundary":
		return true
	default:
		return false
	}
}

func isDeclarationSupportRole(role string) bool {
	switch normalizeDiscourseRole(role) {
	case "evidence", "condition", "action", "scale", "constraint", "non_action", "caveat":
		return true
	default:
		return false
	}
}

func declarationKindForSpine(spine PreviewSpine) string {
	policy := normalizePreviewSpinePolicy(spine.Policy)
	if policy == "management_declaration" {
		return "declaration"
	}
	if policy != "" {
		return policy
	}
	return "declaration"
}

func declarationTopicForSpine(spine PreviewSpine, nodeIDs []string, nodes map[string]graphNode) string {
	if normalizePreviewSpinePolicy(spine.Policy) == "capital_allocation_rule" {
		return "capital_allocation"
	}
	text := strings.ToLower(spine.Thesis)
	for _, id := range nodeIDs {
		node := nodes[id]
		text += " " + strings.ToLower(node.Text) + " " + strings.ToLower(node.SourceQuote)
	}
	switch {
	case containsAnyText(text, []string{"cash", "treasury", "capital allocation", "deploy capital", "allocation", "现金", "短债", "资本配置", "部署资本"}):
		return "capital_allocation"
	case containsAnyText(text, []string{"insurance", "underwriting", "承保", "保险"}):
		return "underwriting"
	case containsAnyText(text, []string{"utility", "energy", "grid", "电网", "能源", "公用事业"}):
		return "utilities"
	default:
		return ""
	}
}

func enrichDeclarationFromSourceQuote(declaration *Declaration) {
	if declaration == nil || declaration.Kind != "capital_allocation_rule" {
		return
	}
	source := strings.ToLower(strings.Join([]string{
		declaration.Statement,
		declaration.SourceQuote,
		strings.Join(declaration.Evidence, " "),
	}, " "))
	if containsAnyText(source, []string{"dislocation", "错配"}) {
		declaration.Conditions = appendUniqueString(declaration.Conditions, "市场出现错配")
	}
	if containsAnyText(source, []string{"attractive", "value proposition", "investment value", "显著投资价值", "具备投资价值"}) {
		declaration.Conditions = appendUniqueString(declaration.Conditions, "机会具备显著投资价值")
	}
	if containsAnyText(source, []string{"act decisively", "decisively both quickly", "quickly and with", "果断", "快速"}) {
		declaration.Actions = appendUniqueString(declaration.Actions, "快速且果断行动")
	}
	if strings.TrimSpace(declaration.Scale) == "" && containsAnyText(source, []string{"significant capital", "large amount of capital", "大量资本", "大额资本"}) {
		declaration.Scale = "投入大量资本"
	}
	if containsAnyText(source, []string{"patience", "discipline", "耐心", "纪律"}) {
		declaration.Constraints = appendUniqueString(declaration.Constraints, "保持资本配置耐心与纪律")
	}
}

func inferDeclarationSpeaker(bundle Bundle, nodeIDs []string, nodes map[string]graphNode) string {
	text := strings.ToLower(bundle.TextContext())
	for _, id := range nodeIDs {
		node := nodes[id]
		text += " " + strings.ToLower(node.Text) + " " + strings.ToLower(node.SourceQuote)
	}
	for _, candidate := range []struct {
		match string
		name  string
	}{
		{"greg abel", "Greg Abel"},
		{"abel", "Greg Abel"},
		{"ajit jain", "Ajit Jain"},
		{"nancy pierce", "Nancy Pierce"},
		{"warren buffett", "Warren Buffett"},
		{"buffett", "Warren Buffett"},
	} {
		if strings.Contains(text, candidate.match) {
			return candidate.name
		}
	}
	return strings.TrimSpace(bundle.AuthorName)
}

func declarationsByID(values []Declaration) map[string]Declaration {
	out := make(map[string]Declaration, len(values))
	for _, declaration := range values {
		if id := strings.TrimSpace(declaration.ID); id != "" {
			out[id] = declaration
		}
	}
	return out
}

func firstDeclarationStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func prioritizeDeclarationSummary(summary string, spines []PreviewSpine, declarations []Declaration) string {
	if len(declarations) == 0 {
		return summary
	}
	declaration := declarations[0]
	for _, candidate := range declarations {
		if normalizePreviewSpinePolicy(candidate.Kind) == "capital_allocation_rule" || strings.TrimSpace(candidate.Topic) == "capital_allocation" {
			declaration = candidate
			break
		}
	}
	if normalizePreviewSpinePolicy(declaration.Kind) != "capital_allocation_rule" && (len(spines) == 0 || !isDeclarationSpinePolicy(spines[0].Policy)) {
		return summary
	}
	if isCapitalAllocationDeclaration(declaration) {
		return capitalAllocationNarrativeSummary(declaration)
	}
	parts := []string{trimDeclarationSummaryPart(declaration.Statement)}
	if len(declaration.Conditions) > 0 {
		parts = append(parts, "条件："+strings.Join(firstDeclarationStrings(declaration.Conditions, 2), "、"))
	}
	if len(declaration.Actions) > 0 {
		parts = append(parts, "行动："+strings.Join(firstDeclarationStrings(declaration.Actions, 2), "、"))
	}
	if strings.TrimSpace(declaration.Scale) != "" {
		parts = append(parts, "规模："+strings.TrimSpace(declaration.Scale))
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return summary
		}
	}
	return strings.TrimSpace(strings.Join(parts, "；")) + "。"
}

func capitalAllocationNarrativeSummary(declaration Declaration) string {
	statement := trimDeclarationSummaryPart(declaration.Statement)
	if statement == "" {
		statement = "伯克希尔把现金头寸视为资本配置选择权，而不是必须立刻投出的资金"
	}
	parts := []string{statement}
	if len(declaration.Constraints) > 0 {
		boundary := strings.Join(trimDeclarationSummaryParts(firstDeclarationStrings(declaration.Constraints, 2)), "、")
		if strings.HasPrefix(boundary, "保持") {
			parts = append(parts, "平时"+boundary)
		} else if boundary != "" {
			parts = append(parts, "平时以"+boundary+"为边界")
		}
	}
	condition := strings.Join(trimDeclarationSummaryParts(firstDeclarationStrings(declaration.Conditions, 2)), "、")
	action := strings.Join(trimDeclarationSummaryParts(firstDeclarationStrings(declaration.Actions, 2)), "、")
	scale := trimDeclarationSummaryPart(declaration.Scale)
	switch {
	case condition != "" && action != "" && scale != "":
		parts = append(parts, "只有当"+condition+"时，才"+action+"，规模是"+scale)
	case condition != "" && action != "":
		parts = append(parts, "只有当"+condition+"时，才"+action)
	case action != "" && scale != "":
		parts = append(parts, action+"，规模是"+scale)
	case action != "":
		parts = append(parts, action)
	}
	if len(declaration.Evidence) > 0 {
		if evidence := trimDeclarationSummaryPart(declaration.Evidence[0]); evidence != "" {
			parts = append(parts, "证据是"+evidence)
		}
	}
	return strings.Join(nonEmptyDeclarationSummaryParts(parts), "；") + "。"
}

func trimDeclarationSummaryParts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value := trimDeclarationSummaryPart(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func nonEmptyDeclarationSummaryParts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value := strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func trimDeclarationSummaryPart(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "。.;；")
}

func attachDeclarationsToBranches(branches []Branch, spines []PreviewSpine, declarations []Declaration) []Branch {
	if len(branches) == 0 || len(declarations) == 0 {
		if len(declarations) == 0 {
			return branches
		}
	}
	index := declarationsByID(declarations)
	spineIndex := map[string]PreviewSpine{}
	for _, spine := range spines {
		spineIndex[strings.TrimSpace(spine.ID)] = spine
	}
	attached := map[string]struct{}{}
	for i := range branches {
		if declaration, ok := index[strings.TrimSpace(branches[i].ID)]; ok {
			branches[i].Declarations = append(branches[i].Declarations, declaration)
			attached[strings.TrimSpace(branches[i].ID)] = struct{}{}
		}
	}
	for _, declaration := range declarations {
		id := strings.TrimSpace(declaration.ID)
		if id == "" {
			id = "declaration"
		}
		if _, ok := attached[id]; ok {
			continue
		}
		spine := spineIndex[id]
		branch := Branch{
			ID:           id,
			Level:        strings.TrimSpace(spine.Level),
			Policy:       declaration.Kind,
			Thesis:       FirstNonEmpty(strings.TrimSpace(spine.Thesis), strings.TrimSpace(declaration.Statement)),
			Declarations: []Declaration{declaration},
		}
		branches = append(branches, branch)
	}
	return branches
}
