package compilev2

import (
	"fmt"
	"hash/fnv"
	"strings"
)

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
