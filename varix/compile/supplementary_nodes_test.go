package compile

import "testing"

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
	got := splitParallelDrivers([]string{"全球石油及相关商品库存持续消耗"})
	if len(got) != 1 || got[0] != "全球石油及相关商品库存持续消耗" {
		t.Fatalf("Drivers = %#v, want no fragment split", got)
	}
}

func TestAlignTransmissionPathDriversSplitsMergedPathDriverToTopLevelDrivers(t *testing.T) {
	paths := alignTransmissionPathDrivers(
		[]string{"市场定价美伊冲突最坏情景未现", "美元资产流动性得以维持"},
		[]TransmissionPath{{
			Driver: "市场定价美伊冲突最坏情景未现且美元资产流动性得以维持",
			Target: "美股反弹进入超买区间且风险收益比转向不对称下行",
			Steps:  []string{"资金重新增加杠杆推动股市与战事脱敏"},
		}},
	)
	if len(paths) != 2 {
		t.Fatalf("len(paths) = %d, want 2", len(paths))
	}
	if paths[0].Driver == paths[1].Driver {
		t.Fatalf("aligned drivers = %#v, want one path per top-level driver", paths)
	}
}
