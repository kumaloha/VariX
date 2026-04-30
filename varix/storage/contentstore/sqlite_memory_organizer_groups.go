package contentstore

import (
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

type canonicalNodeGroup struct {
	canonical string
	ids       []string
}

func buildDedupeGroups(nodes []memory.AcceptedNode, _ map[string]compile.FactStatus, _ map[string]compile.PredictionStatus) []memory.DedupeGroup {
	groups := groupNodesByCanonicalText(nodes)
	out := make([]memory.DedupeGroup, 0, len(groups))
	for _, group := range groups {
		if len(group.ids) <= 1 {
			continue
		}
		ids := cloneStringSlice(group.ids)
		out = append(out, memory.DedupeGroup{
			NodeIDs:              ids,
			RepresentativeNodeID: ids[0],
			CanonicalText:        group.canonical,
			Reason:               "canonicalized text match",
			Hint:                 "merge-near-duplicate",
		})
	}
	return out
}

func buildContradictionGroups(nodes []memory.AcceptedNode) []memory.ContradictionGroup {
	groups := groupNodesByCanonicalText(nodes)
	out := make([]memory.ContradictionGroup, 0)
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			reason, ok := contradictionReason(groups[i].canonical, groups[j].canonical)
			if !ok {
				continue
			}
			ids := append(cloneStringSlice(groups[i].ids), groups[j].ids...)
			sort.Strings(ids)
			out = append(out, memory.ContradictionGroup{
				NodeIDs: ids,
				Reason:  reason,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return joinNodeIDs(out[i].NodeIDs) < joinNodeIDs(out[j].NodeIDs)
	})
	return out
}

func groupNodesByCanonicalText(nodes []memory.AcceptedNode) []canonicalNodeGroup {
	byText := map[string][]string{}
	for _, node := range nodes {
		key := canonicalNodeText(node.NodeText)
		byText[key] = append(byText[key], node.NodeID)
	}
	keys := make([]string, 0, len(byText))
	for key := range byText {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]canonicalNodeGroup, 0, len(keys))
	for _, key := range keys {
		ids := cloneStringSlice(byText[key])
		sort.Strings(ids)
		out = append(out, canonicalNodeGroup{
			canonical: key,
			ids:       ids,
		})
	}
	return out
}

func normalizeNodeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func canonicalNodeText(text string) string {
	normalized := normalizeNodeText(text)
	replacer := strings.NewReplacer(
		"，", "",
		"。", "",
		"！", "",
		"？", "",
		"!", "",
		"?", "",
		".", "",
		",", "",
		"：", "",
		"；", "",
		"、", "",
		"“", "",
		"”", "",
		"如果", "",
		"若", "",
		"一旦", "",
		"假如", "",
		"倘若", "",
		"如若", "",
		"发生", "",
		"（", "",
		"）", "",
		"(", "",
		")", "",
		"继续", "",
		"仍", "",
		"预计", "",
		"预期", "",
		"可能", "",
		"有望", "",
		"正在", "",
		"会", "",
		"将", "",
		"将会", "",
		"走高", "上升",
		"上涨", "上升",
		"攀升", "上升",
		"回升", "上升",
		"下滑", "下降",
		"下跌", "下降",
		"走低", "下降",
		"回落", "下降",
		"上行", "上升",
		"下行", "下降",
		"走弱", "下降",
		"走强", "上升",
		"减弱", "削弱",
		"强化", "增强",
		"支撑", "支持",
	)
	return replacer.Replace(normalized)
}

func sameNodeMeaning(a, b string) bool {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func areContradictory(a, b string) bool {
	_, ok := contradictionReason(a, b)
	return ok
}

func contradictionReason(a, b string) (string, bool) {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if strings.ReplaceAll(a, "不", "") == b || strings.ReplaceAll(b, "不", "") == a {
		return "negation contradiction", true
	}
	if strings.ReplaceAll(a, "不会", "") == b || strings.ReplaceAll(b, "不会", "") == a {
		return "negation contradiction", true
	}
	for _, pair := range [][2]string{
		{"上升", "下降"},
		{"增加", "减少"},
		{"恶化", "改善"},
		{"紧张", "缓和"},
		{"收缩", "扩张"},
		{"宽松", "收紧"},
		{"削弱", "增强"},
		{"利多", "利空"},
		{"支持", "压制"},
		{"升温", "降温"},
	} {
		if strings.ReplaceAll(a, pair[0], pair[1]) == b || strings.ReplaceAll(a, pair[1], pair[0]) == b {
			return "antonym contradiction", true
		}
		if strings.ReplaceAll(b, pair[0], pair[1]) == a || strings.ReplaceAll(b, pair[1], pair[0]) == a {
			return "antonym contradiction", true
		}
	}
	return "", false
}

func joinNodeIDs(ids []string) string {
	return strings.Join(ids, "\x00")
}
