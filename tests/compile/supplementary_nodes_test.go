package compile

import (
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/model"
)

func TestParseOutputKeepsSupplementaryNodesAndSplitsParallelDrivers(t *testing.T) {
	out, err := ParseOutput(`{
	  "summary":"一句话",
	  "drivers":["美伊停战预期和流动性风险可控预期"],
	  "targets":["美股进入超买"],
	  "transmission_paths":[{"driver":"美伊停战预期","target":"美股进入超买","steps":["市场提前定价"]}],
	  "evidence_nodes":["RSI升至70"],
	  "explanation_nodes":["真实停战仍未证实"],
	  "supplementary_nodes":["“市场提前按停战定价”更像对driver的补充说明"],
	  "graph":{"nodes":[
	    {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-19T00:00:00Z"},
	    {"id":"n2","kind":"结论","text":"结论B"}
	  ],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},
	  "details":{"caveats":["detail"]},
	  "topics":["topic"],
	  "confidence":"medium"
	}`)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if len(out.Drivers) != 2 {
		t.Fatalf("Drivers = %#v, want split parallel drivers", out.Drivers)
	}
	if len(out.SupplementaryNodes) != 1 {
		t.Fatalf("SupplementaryNodes = %#v, want 1", out.SupplementaryNodes)
	}
}

func TestSplitParallelDriversDoesNotProduceNounFragments(t *testing.T) {
	got := model.SplitParallelDrivers([]string{"全球石油及相关商品库存持续消耗"})
	if len(got) != 1 || got[0] != "全球石油及相关商品库存持续消耗" {
		t.Fatalf("Drivers = %#v, want no fragment split", got)
	}
}

func TestParseOutputExtractsFirstJSONObjectFromWrappedResponse(t *testing.T) {
	out, err := ParseOutput(`说明文字
{
  "summary":"一句话",
  "drivers":["资金流入美国资产"],
  "targets":["海外资金继续流入美国资产"],
  "transmission_paths":[{"driver":"资金流入美国资产","target":"海外资金继续流入美国资产","steps":["资金继续配置美国资产"]}],
  "evidence_nodes":["TIC显示资金净流入"],
  "explanation_nodes":["市场仍偏好美国资产"],
  "supplementary_nodes":["没有形成 sell America 交易"],
  "graph":{"nodes":[
    {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-19T00:00:00Z"},
    {"id":"n2","kind":"结论","text":"结论B"}
  ],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},
  "details":{"caveats":["detail"]},
  "topics":["topic"],
  "confidence":"medium"
}
多余尾巴`)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if got := out.Summary; got != "一句话" {
		t.Fatalf("Summary = %q, want 一句话", got)
	}
}

func TestParseOutputRepairsSuspiciousInnerQuotesInJSONString(t *testing.T) {
	out, err := ParseOutput(`{
  "summary":"文章驳斥了"卖出美国"和"对冲美国"叙事。",
  "drivers":["美联储政治化"],
  "targets":["美元走弱"],
  "transmission_paths":[{"driver":"美联储政治化","target":"美元走弱","steps":["压低实际收益率"]}],
  "evidence_nodes":["TIC数据显示外资净流入美国资产创25年新高"],
  "explanation_nodes":["美元并非因资本外逃而走弱"],
  "supplementary_nodes":["没有形成 sell America 交易"],
  "graph":{"nodes":[
    {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-19T00:00:00Z"},
    {"id":"n2","kind":"结论","text":"结论B"}
  ],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},
  "details":{"caveats":["detail"]},
  "topics":["topic"],
  "confidence":"medium"
}`)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if !strings.Contains(out.Summary, "卖出美国") {
		t.Fatalf("Summary = %q, want repaired quoted phrase", out.Summary)
	}
}
