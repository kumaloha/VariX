package compile

import (
	"fmt"
	"strconv"
	"strings"
)

func BuildMainlineMarkdown(result FlowPreviewResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s:%s\n\n", result.Platform, result.ExternalID)
	fmt.Fprintf(&b, "URL: %s\n\n", result.URL)
	fmt.Fprintf(&b, "Drivers: %d\n", len(result.Render.Drivers))
	fmt.Fprintf(&b, "Targets: %d\n", len(result.Render.Targets))
	fmt.Fprintf(&b, "Paths: %d\n\n", len(result.Render.TransmissionPaths))
	if len(result.Spines) > 0 {
		b.WriteString("Spines:\n")
		for _, spine := range result.Spines {
			family := ""
			if strings.TrimSpace(spine.FamilyKey) != "" {
				family = fmt.Sprintf(" [%s/%s/%s]", spine.FamilyKey, spine.FamilyLabel, spine.FamilyScope)
			}
			fmt.Fprintf(&b, "- Spine %s (%s, priority %d)%s: %s\n", spine.ID, spine.Level, spine.Priority, family, spine.Thesis)
		}
		b.WriteString("\n")
	}
	b.WriteString("```mermaid\nflowchart LR\n")
	if len(result.Spines) > 0 {
		writePreviewSpinesMermaid(&b, relationLabelMap(result.Relations), result.Spines)
		b.WriteString("```\n")
		return b.String()
	}
	if len(result.Relations.Edges) > 0 {
		writePreviewEdgesMermaid(&b, relationLabelMap(result.Relations), result.Relations.Edges)
		b.WriteString("```\n")
		return b.String()
	}
	if len(result.Render.TransmissionPaths) == 0 {
		b.WriteString("  n0[\"No mainline path\"]\n")
		b.WriteString("```\n")
		return b.String()
	}
	labelToID := map[string]string{}
	nextID := 1
	nodeID := func(label string) string {
		label = strings.TrimSpace(label)
		if label == "" {
			label = "Unnamed"
		}
		if id, ok := labelToID[label]; ok {
			return id
		}
		id := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = id
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, escapeMermaidLabel(label))
		return id
	}
	for _, path := range result.Render.TransmissionPaths {
		prev := nodeID(path.Driver)
		prevLabel := strings.TrimSpace(path.Driver)
		for _, step := range path.Steps {
			step = strings.TrimSpace(step)
			if step == "" || strings.EqualFold(step, prevLabel) {
				continue
			}
			cur := nodeID(step)
			fmt.Fprintf(&b, "  %s --> %s\n", prev, cur)
			prev = cur
			prevLabel = step
		}
		target := nodeID(path.Target)
		if !strings.EqualFold(strings.TrimSpace(path.Target), prevLabel) {
			fmt.Fprintf(&b, "  %s --> %s\n", prev, target)
		}
	}
	b.WriteString("```\n")
	return b.String()
}

func relationLabelMap(graph PreviewGraph) map[string]string {
	labels := map[string]string{}
	for _, node := range graph.Nodes {
		labels[node.ID] = node.Text
	}
	return labels
}

func collectSpineEdges(spines []PreviewSpine) []PreviewEdge {
	var out []PreviewEdge
	for _, spine := range spines {
		out = append(out, spine.Edges...)
	}
	return out
}

func writePreviewSpinesMermaid(b *strings.Builder, labels map[string]string, spines []PreviewSpine) {
	labelToID := map[string]string{}
	nextID := 1
	nodeIDForLabel := func(label string) string {
		if label == "" {
			label = "Unnamed"
		}
		if existing, ok := labelToID[label]; ok {
			return existing
		}
		renderID := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = renderID
		fmt.Fprintf(b, "  %s[\"%s\"]\n", renderID, escapeMermaidLabel(label))
		return renderID
	}
	nodeID := func(id string) string {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			label = strings.TrimSpace(id)
		}
		return nodeIDForLabel(label)
	}
	renderedNodes := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	for _, spine := range spines {
		if isSatiricalPreviewSpine(spine) && writeSatiricalPreviewSpineMermaid(b, labels, spine, nodeIDForLabel, renderedNodes, seenEdges) {
			continue
		}
		for _, edge := range spine.Edges {
			if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
				continue
			}
			from := nodeID(edge.From)
			to := nodeID(edge.To)
			renderedNodes[edge.From] = struct{}{}
			renderedNodes[edge.To] = struct{}{}
			if from == to {
				continue
			}
			key := from + "->" + to
			if _, ok := seenEdges[key]; ok {
				continue
			}
			seenEdges[key] = struct{}{}
			fmt.Fprintf(b, "  %s --> %s\n", from, to)
		}
	}
	for _, spine := range spines {
		for _, id := range spine.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := renderedNodes[id]; ok {
				continue
			}
			nodeID(id)
			renderedNodes[id] = struct{}{}
		}
	}
}

func isSatiricalPreviewSpine(spine PreviewSpine) bool {
	return strings.EqualFold(strings.TrimSpace(spine.Policy), "satirical_analogy")
}

func writeSatiricalPreviewSpineMermaid(
	b *strings.Builder,
	labels map[string]string,
	spine PreviewSpine,
	nodeIDForLabel func(string) string,
	renderedNodes map[string]struct{},
	seenEdges map[string]struct{},
) bool {
	vehicle, ok := satiricalVehicleLabel(labels, spine)
	if !ok {
		return false
	}
	mechanism := satiricalMechanismLabel(labels, spine)
	conclusion := satiricalConclusionLabel(labels, spine)
	if mechanism == "" && conclusion == "" {
		mechanism = strings.TrimSpace(spine.Thesis)
	}
	if mechanism == "" && conclusion == "" {
		return false
	}
	vehicleID := nodeIDForLabel(vehicle)
	prevID := vehicleID
	for _, label := range []string{mechanism, conclusion} {
		label = strings.TrimSpace(label)
		if label == "" || strings.EqualFold(label, vehicle) {
			continue
		}
		nextID := nodeIDForLabel(label)
		if nextID != prevID {
			key := prevID + "->" + nextID
			if _, ok := seenEdges[key]; !ok {
				seenEdges[key] = struct{}{}
				fmt.Fprintf(b, "  %s --> %s\n", prevID, nextID)
			}
		}
		prevID = nextID
	}
	for _, id := range spine.NodeIDs {
		if strings.TrimSpace(id) != "" {
			renderedNodes[id] = struct{}{}
		}
	}
	return true
}

func satiricalVehicleLabel(labels map[string]string, spine PreviewSpine) (string, bool) {
	for _, id := range spine.NodeIDs {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			continue
		}
		lower := strings.ToLower(label)
		if strings.Contains(label, "幸运游戏") {
			return "讽刺寓言：幸运游戏", true
		}
		if strings.Contains(label, "寓言") || strings.Contains(label, "类比") || strings.Contains(lower, "allegory") || strings.Contains(lower, "analogy") {
			return "讽刺寓言：" + compactPreviewLabel(label, 32), true
		}
	}
	for _, id := range spine.NodeIDs {
		if label := strings.TrimSpace(labels[id]); label != "" {
			return "讽刺寓言：" + compactPreviewLabel(label, 32), true
		}
	}
	return "", false
}

func satiricalMechanismLabel(labels map[string]string, spine PreviewSpine) string {
	if label := satiricalBenefitLossLabel(labels, spine); label != "" {
		return "映射机制：" + compactPreviewLabel(label, 42)
	}
	for _, markerSet := range [][]string{
		{"包装", "公平"},
		{"内部", "赢家"},
		{"吸引", "75"},
		{"忽悠"},
		{"不公平"},
		{"机制"},
	} {
		if label := firstSatiricalLabelMatching(labels, spine, markerSet, false); label != "" {
			return "映射机制：" + compactPreviewLabel(label, 42)
		}
	}
	return ""
}

func satiricalBenefitLossLabel(labels map[string]string, spine PreviewSpine) string {
	best := ""
	bestScore := -1
	for _, id := range spine.NodeIDs {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			continue
		}
		if containsAnyText(label, []string{"受益", "赚", "收益", "赢家"}) &&
			containsAnyText(label, []string{"后75", "75%", "净亏", "承担", "损失", "成本", "买单"}) {
			score := satiricalBenefitLossScore(label)
			if score > bestScore {
				best = label
				bestScore = score
			}
		}
	}
	return best
}

func satiricalBenefitLossScore(label string) int {
	score := 0
	for _, marker := range []string{"受益", "净亏", "损失", "承担", "成本", "买单", "管理费", "手续费", "本金", "转移"} {
		if strings.Contains(label, marker) {
			score += 3
		}
	}
	for _, marker := range []string{"前25", "25%", "后75", "75%", "赢家"} {
		if strings.Contains(label, marker) {
			score++
		}
	}
	return score
}

func satiricalConclusionLabel(labels map[string]string, spine PreviewSpine) string {
	for _, markerSet := range [][]string{
		{"现实", "骗局"},
		{"风险识别"},
		{"不是智商"},
		{"净亏"},
		{"承担", "成本"},
		{"买单"},
	} {
		if label := firstSatiricalLabelMatching(labels, spine, markerSet, true); label != "" {
			return "批判结论：" + compactPreviewLabel(label, 42)
		}
	}
	return ""
}

func firstSatiricalLabelMatching(labels map[string]string, spine PreviewSpine, markers []string, preferLast bool) string {
	matches := make([]string, 0, 1)
	for _, id := range spine.NodeIDs {
		label := strings.TrimSpace(labels[id])
		if label == "" || isSatiricalStoryDetail(label) {
			continue
		}
		ok := true
		for _, marker := range markers {
			if !strings.Contains(label, marker) {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, label)
		}
	}
	if len(matches) == 0 {
		return ""
	}
	if preferLast {
		return matches[len(matches)-1]
	}
	return matches[0]
}

func isSatiricalStoryDetail(label string) bool {
	for _, marker := range []string{"每月", "每人", "利率", "委托贷款", "本金贷回", "抽一人", "抽完", "中奖者"} {
		if strings.Contains(label, marker) {
			return true
		}
	}
	return false
}

func compactPreviewLabel(label string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(label))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func writePreviewEdgesMermaid(b *strings.Builder, labels map[string]string, edges []PreviewEdge) {
	labelToID := map[string]string{}
	nextID := 1
	nodeID := func(id string) string {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			label = strings.TrimSpace(id)
		}
		if label == "" {
			label = "Unnamed"
		}
		if existing, ok := labelToID[label]; ok {
			return existing
		}
		renderID := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = renderID
		fmt.Fprintf(b, "  %s[\"%s\"]\n", renderID, escapeMermaidLabel(label))
		return renderID
	}
	seenEdges := map[string]struct{}{}
	for _, edge := range edges {
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
			continue
		}
		from := nodeID(edge.From)
		to := nodeID(edge.To)
		if from == to {
			continue
		}
		key := from + "->" + to
		if _, ok := seenEdges[key]; ok {
			continue
		}
		seenEdges[key] = struct{}{}
		fmt.Fprintf(b, "  %s --> %s\n", from, to)
	}
}
