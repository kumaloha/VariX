package compile

import (
	"fmt"
	"strings"
)

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
		path.branchID = spine.ID
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
