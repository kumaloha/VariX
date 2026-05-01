package compilev2

import (
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func normalizeStage1State(state graphState) graphState {
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		n.ID = strings.TrimSpace(n.ID)
		n.Text = strings.TrimSpace(n.Text)
		n.SourceQuote = strings.TrimSpace(n.SourceQuote)
		n.DiscourseRole = normalizeDiscourseRole(n.DiscourseRole)
		if n.Text == "" {
			continue
		}
		nodes = append(nodes, n)
	}
	state.Nodes = nodes
	state.ArticleForm = normalizeArticleForm(state.ArticleForm)

	validIDs := map[string]struct{}{}
	for _, n := range state.Nodes {
		validIDs[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		e.From = strings.TrimSpace(e.From)
		e.To = strings.TrimSpace(e.To)
		if e.From == "" || e.To == "" || e.From == e.To {
			continue
		}
		if _, ok := validIDs[e.From]; !ok {
			continue
		}
		if _, ok := validIDs[e.To]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = edges

	off := make([]offGraphItem, 0, len(state.OffGraph))
	for _, o := range state.OffGraph {
		o.Text = strings.TrimSpace(o.Text)
		if o.Text == "" {
			continue
		}
		o.Role = strings.TrimSpace(o.Role)
		if o.Role == "" {
			o.Role = "supplementary"
		}
		off = append(off, o)
	}
	state.OffGraph = off
	return state
}

func decodeStage1Nodes(raw any) []graphNode {
	items, _ := raw.([]any)
	out := make([]graphNode, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, graphNode{Text: strings.TrimSpace(v)})
		case map[string]any:
			out = append(out, graphNode{
				ID:            strings.TrimSpace(asString(v["id"])),
				Text:          strings.TrimSpace(compile.FirstNonEmpty(asString(v["text"]), asString(v["content"]))),
				SourceQuote:   strings.TrimSpace(asString(v["source_quote"])),
				DiscourseRole: normalizeDiscourseRole(asString(v["role"])),
			})
		}
	}
	return out
}

func decodeStage1Edges(raw any) []graphEdge {
	items, _ := raw.([]any)
	out := make([]graphEdge, 0, len(items))
	for _, item := range items {
		v, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, graphEdge{
			From: strings.TrimSpace(compile.FirstNonEmpty(asString(v["from"]), asString(v["source"]))),
			To:   strings.TrimSpace(compile.FirstNonEmpty(asString(v["to"]), asString(v["target"]))),
		})
	}
	return out
}

func decodeStage1OffGraph(raw any) []offGraphItem {
	items, _ := raw.([]any)
	out := make([]offGraphItem, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, offGraphItem{Text: strings.TrimSpace(v), Role: "supplementary"})
		case map[string]any:
			out = append(out, offGraphItem{
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(asString(v["text"])),
				Role:        strings.TrimSpace(asString(v["role"])),
				AttachesTo:  strings.TrimSpace(asString(v["attaches_to"])),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
}

func fillMissingStage1IDs(state graphState) graphState {
	for i := range state.Nodes {
		fillMissingStage1Identity(&state.Nodes[i].ID, fmt.Sprintf("n%d", i+1))
		fillMissingStage1Text(&state.Nodes[i].SourceQuote, state.Nodes[i].Text)
	}
	for i := range state.OffGraph {
		fillMissingStage1Identity(&state.OffGraph[i].ID, fmt.Sprintf("o%d", i+1))
		if strings.TrimSpace(state.OffGraph[i].Role) == "" {
			state.OffGraph[i].Role = "supplementary"
		}
		fillMissingStage1Text(&state.OffGraph[i].SourceQuote, state.OffGraph[i].Text)
	}
	return state
}

func fillMissingStage1Identity(field *string, fallback string) {
	if field == nil || strings.TrimSpace(*field) != "" {
		return
	}
	*field = fallback
}

func fillMissingStage1Text(field *string, fallback string) {
	if field == nil || strings.TrimSpace(*field) != "" {
		return
	}
	*field = fallback
}
