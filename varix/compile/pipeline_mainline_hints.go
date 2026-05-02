package compile

import (
	"fmt"
	"strings"
)

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
