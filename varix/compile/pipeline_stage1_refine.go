package compile

import (
	"fmt"
	"strings"
)

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
	Text          string `json:"text"`
	SourceQuote   string `json:"source_quote"`
	DiscourseRole string `json:"role"`
}

type refineReplacementEdge struct {
	FromIndex   int    `json:"from_index"`
	ToIndex     int    `json:"to_index"`
	Kind        string `json:"kind"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
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
				ID:            fmt.Sprintf("%s_%d", replaceID, idx+1),
				Text:          text,
				SourceQuote:   sourceQuote,
				DiscourseRole: normalizeDiscourseRole(item.DiscourseRole),
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
			if strings.TrimSpace(nodes[i].DiscourseRole) == "" {
				nodes[i].DiscourseRole = node.DiscourseRole
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
