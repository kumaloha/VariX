package compile

import "strings"

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
