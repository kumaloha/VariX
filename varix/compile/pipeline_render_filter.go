package compile

import (
	"fmt"
	"sort"
	"strings"
)

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
