package compile

import (
	"fmt"
	"strings"
)

func stageLedger(state graphState) graphState {
	if len(state.Ledger.Items) > 0 {
		return state
	}
	state.Ledger = buildLedger(state)
	return state
}

func buildLedger(state graphState) Ledger {
	items := make([]LedgerItem, 0, len(state.SemanticUnits)+len(state.Nodes)+len(state.OffGraph))
	if len(state.SemanticUnits) > 0 {
		for _, unit := range rankSemanticUnits(state.SemanticUnits, "") {
			items = appendLedgerItem(items, ledgerItemFromSemanticUnit(unit))
		}
	}
	for _, node := range state.Nodes {
		items = appendLedgerItem(items, ledgerItemFromGraphNode(node))
	}
	for _, item := range state.OffGraph {
		items = appendLedgerItem(items, ledgerItemFromOffGraph(item))
	}
	return Ledger{Items: items}
}

func appendLedgerItem(items []LedgerItem, item LedgerItem) []LedgerItem {
	if strings.TrimSpace(item.Claim) == "" {
		return items
	}
	key := normalizeText(item.Claim)
	for _, existing := range items {
		if normalizeText(existing.Claim) == key {
			return items
		}
	}
	item.ID = fmt.Sprintf("ledger-%03d", len(items)+1)
	items = append(items, item)
	return items
}

func ledgerItemFromSemanticUnit(unit SemanticUnit) LedgerItem {
	text := strings.Join([]string{unit.Subject, unit.Force, unit.Claim, unit.PromptContext}, " ")
	item := LedgerItem{
		Kind:      ledgerKind(text),
		Category:  ledgerCategory(text),
		Claim:     strings.TrimSpace(unit.Claim),
		Entities:  ledgerEntities(text),
		Numbers:   ledgerNumbers(text),
		Quote:     strings.TrimSpace(unit.SourceQuote),
		SourceIDs: []string{strings.TrimSpace(unit.ID)},
		Salience:  unit.Salience,
	}
	if item.Kind == "" {
		item.Kind = "claim"
	}
	return item
}

func ledgerItemFromGraphNode(node graphNode) LedgerItem {
	text := strings.Join([]string{node.Text, node.DiscourseRole, node.Ontology}, " ")
	return LedgerItem{
		Kind:      ledgerKind(text),
		Category:  ledgerCategory(text),
		Claim:     strings.TrimSpace(node.Text),
		Entities:  ledgerEntities(text),
		Numbers:   ledgerNumbers(text),
		Quote:     strings.TrimSpace(node.SourceQuote),
		SourceIDs: nonEmptyLedgerSourceIDs(node.ID),
		Salience:  0.5,
	}
}

func ledgerItemFromOffGraph(item offGraphItem) LedgerItem {
	text := strings.Join([]string{item.Text, item.Role}, " ")
	return LedgerItem{
		Kind:      ledgerKind(text),
		Category:  ledgerCategory(text),
		Claim:     strings.TrimSpace(item.Text),
		Entities:  ledgerEntities(text),
		Numbers:   ledgerNumbers(text),
		Quote:     strings.TrimSpace(item.SourceQuote),
		SourceIDs: nonEmptyLedgerSourceIDs(item.ID),
		Salience:  0.4,
	}
}

func nonEmptyLedgerSourceIDs(ids ...string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func ledgerKind(text string) string {
	lower := strings.ToLower(text)
	switch {
	case len(ledgerEntities(text)) >= 3:
		return "list"
	case containsAnyText(lower, []string{"will not", "不会", "拒绝", "避免", "boundary", "边界"}):
		return "boundary"
	case containsAnyText(lower, []string{"risk", "litigation", "lawsuit", "风险", "诉讼"}):
		return "risk"
	case containsAnyText(lower, []string{"commit", "plan", "will", "计划", "承诺"}):
		return "commitment"
	case containsAnyText(lower, []string{"disclose", "披露"}):
		return "disclosure"
	case len(ledgerNumbers(text)) > 0:
		return "number"
	default:
		return "claim"
	}
}

func ledgerCategory(text string) string {
	lower := strings.ToLower(text)
	switch {
	case containsAnyText(lower, []string{"buyback", "repurchase", "回购"}):
		return "buyback"
	case containsAnyText(lower, []string{"portfolio", "holding", "apple", "coca-cola", "american express", "bank of america", "trading house", "现有组合", "股票组合", "持仓", "能力圈"}):
		return "portfolio"
	case containsAnyText(lower, []string{"succession", "successor", "继任", "接班"}):
		return "succession"
	case containsAnyText(lower, []string{"culture", "values", "bureaucracy", "current form", "文化", "价值观", "官僚", "现有形式"}):
		return "culture"
	case containsAnyText(lower, []string{"data center", "energy", "utility", "utilities", "electric", "数据中心", "能源", "电力", "公用事业"}):
		return "energy"
	case containsAnyText(lower, []string{"capital allocation", "cash", "treasury", "资本配置", "现金", "国债", "美债"}):
		return "capital"
	case containsAIReference(lower) || containsAnyText(lower, []string{"artificial intelligence", "人工智能", "大模型"}):
		return "ai"
	case containsAnyText(lower, []string{"cyber", "insurance", "underwriting", "geico", "float", "网络", "保险", "承保", "浮存金", "保单", "理赔", "风险定价"}):
		return "insurance"
	case containsAnyText(lower, []string{"bnsf", "clayton", "rail", "railroad", "margin", "operating plan", "cost reduction", "铁路", "克莱顿", "建筑", "运营", "经营"}):
		return "operations"
	case containsAnyText(lower, []string{"tariff", "trade war", "关税"}):
		return "macro"
	case containsAnyText(lower, []string{"canada", "canadian", "shareholder", "股东", "加拿大"}):
		return "shareholder"
	default:
		return "governance"
	}
}

func ledgerEntities(text string) []string {
	return briefEntities(text)
}

func ledgerNumbers(text string) []string {
	return briefNumbers(text)
}
