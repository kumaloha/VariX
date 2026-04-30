package compilev2

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

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
