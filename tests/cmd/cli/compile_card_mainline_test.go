package main

import (
	"strings"
	"testing"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
)

func TestFormatCompileCardSurfacesPrimaryDeclarationMainlineBeforeSidePaths(t *testing.T) {
	record := c.Record{
		UnitID:     "youtube:mainline",
		Source:     "youtube",
		ExternalID: "mainline",
		Model:      varixllm.Qwen36PlusModel,
		Output: c.Output{
			Summary: "一句话总结",
			Drivers: []string{"市场犹如附带赌场的教堂"},
			Targets: []string{"市场参与者情绪已转向极端赌博心态"},
			Declarations: []c.Declaration{{
				ID:          "decl-primary",
				Speaker:     "Warren Buffett",
				Kind:        "capital_allocation_rule",
				Topic:       "capital_allocation",
				Statement:   "当前市场环境不利于伯克希尔配置现金",
				Conditions:  []string{"突然出现市场错配"},
				Actions:     []string{"快速部署现金"},
				Constraints: []string{"价格过高时保持等待"},
			}},
			TransmissionPaths: []c.TransmissionPath{{
				Driver: "市场犹如附带赌场的教堂",
				Target: "市场参与者情绪已转向极端赌博心态",
			}},
			Branches: []c.Branch{
				{
					ID:            "s3",
					Level:         "branch",
					Policy:        "causal_mechanism",
					Thesis:        "当前市场结构已由投资转向赌场式赌博",
					BranchDrivers: []string{"市场犹如附带赌场的教堂"},
					Targets:       []string{"市场参与者情绪已转向极端赌博心态"},
					TransmissionPaths: []c.TransmissionPath{{
						Driver: "市场犹如附带赌场的教堂",
						Target: "市场参与者情绪已转向极端赌博心态",
					}},
				},
				{
					ID:     "s1",
					Level:  "primary",
					Policy: "capital_allocation_rule",
					Thesis: "伯克希尔在价格过高、理解不足或环境不理想时持有现金，并等待突然出现的错配机会。",
					Declarations: []c.Declaration{{
						ID:          "decl-primary",
						Speaker:     "Warren Buffett",
						Kind:        "capital_allocation_rule",
						Topic:       "capital_allocation",
						Statement:   "当前市场环境不利于伯克希尔配置现金",
						Conditions:  []string{"突然出现市场错配"},
						Actions:     []string{"快速部署现金"},
						Constraints: []string{"价格过高时保持等待"},
					}},
				},
			},
			Topics: []string{
				"当前市场结构已由投资转向赌场式赌博",
				"伯克希尔在价格过高、理解不足或环境不理想时持有现金",
			},
			Confidence: "medium",
		},
		CompiledAt: time.Now().UTC(),
	}

	out := formatCompileCard(buildCompileCardProjection(record, nil))
	for _, want := range []string{
		"Mainline",
		"Declaration: 当前市场环境不利于伯克希尔配置现金",
		"Read: 这是伯克希尔的资本配置纪律",
		"Side logic",
		"市场犹如附带赌场的教堂 -> 市场参与者情绪已转向极端赌博心态",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "会怎么用手里的钱") {
		t.Fatalf("stdout = %q, did not want capital allocation read tied to a speaker's personal money", out)
	}
	if strings.Contains(out, "Logic chain") {
		t.Fatalf("stdout = %q, want secondary paths labeled Side logic when a primary mainline exists", out)
	}
	if strings.Index(out, "Mainline") > strings.Index(out, "Side logic") {
		t.Fatalf("stdout = %q, want primary mainline before secondary side logic", out)
	}
	topicsStart := strings.Index(out, "Topics")
	if topicsStart < 0 {
		t.Fatalf("stdout = %q, want topics section", out)
	}
	topicsEnd := strings.Index(out[topicsStart:], "Management declarations")
	if topicsEnd < 0 {
		t.Fatalf("stdout = %q, want section after topics", out)
	}
	topicSection := out[topicsStart : topicsStart+topicsEnd]
	if strings.Index(topicSection, "当前市场环境不利于伯克希尔配置现金") > strings.Index(topicSection, "当前市场结构已由投资转向赌场式赌博") {
		t.Fatalf("stdout = %q, want primary topic before secondary topic", out)
	}
	branchesStart := strings.Index(out, "Branches")
	if branchesStart < 0 {
		t.Fatalf("stdout = %q, want branch section", out)
	}
	branchSection := out[branchesStart:]
	if strings.Index(branchSection, "当前市场环境不利于伯克希尔配置现金") > strings.Index(branchSection, "当前市场结构已由投资转向赌场式赌博") {
		t.Fatalf("stdout = %q, want primary branch before secondary branch in branch section", out)
	}
}

func TestFormatCompileCardRendersPrimaryPathAsMainline(t *testing.T) {
	record := c.Record{
		UnitID:     "web:path-mainline",
		Source:     "web",
		ExternalID: "path-mainline",
		Model:      varixllm.Qwen36PlusModel,
		Output: c.Output{
			Summary: "一句话总结",
			Drivers: []string{"政策收紧"},
			Targets: []string{"融资成本上升"},
			TransmissionPaths: []c.TransmissionPath{{
				Driver: "政策收紧",
				Steps:  []string{"银行风险偏好下降"},
				Target: "融资成本上升",
			}},
			Branches: []c.Branch{{
				ID:     "s1",
				Level:  "primary",
				Policy: "causal_mechanism",
				Thesis: "政策收紧通过银行风险偏好影响融资成本。",
				TransmissionPaths: []c.TransmissionPath{{
					Driver: "政策收紧",
					Steps:  []string{"银行风险偏好下降"},
					Target: "融资成本上升",
				}},
			}},
			Confidence: "high",
		},
		CompiledAt: time.Now().UTC(),
	}

	out := formatCompileCard(buildCompileCardProjection(record, nil))
	for _, want := range []string{
		"Mainline",
		"Thesis: 政策收紧通过银行风险偏好影响融资成本。",
		"Path: 政策收紧 -> 银行风险偏好下降 -> 融资成本上升",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "Side logic") || strings.Contains(out, "Logic chain") {
		t.Fatalf("stdout = %q, did not want primary path duplicated as a secondary logic section", out)
	}
}

func TestBuildCompileCardProjectionDoesNotRepeatGraphFirstPrimaryPathAsSideLogic(t *testing.T) {
	record := c.Record{
		UnitID:     "web:graph-first-mainline",
		Source:     "web",
		ExternalID: "graph-first-mainline",
		Model:      varixllm.Qwen36PlusModel,
		Output: c.Output{
			Summary: "一句话总结",
			Branches: []c.Branch{{
				ID:     "s1",
				Level:  "primary",
				Policy: "causal_mechanism",
				Thesis: "AI基建资本开支激增主导市场叙事",
				TransmissionPaths: []c.TransmissionPath{{
					Driver: "AI资本开支激增",
					Steps:  []string{"AI叙事压倒宏观冲击"},
					Target: "纳斯达克上涨",
				}},
			}},
			TransmissionPaths: []c.TransmissionPath{{
				Driver: "AI资本开支激增",
				Steps:  []string{"AI叙事压倒宏观冲击"},
				Target: "纳斯达克上涨",
			}},
			Confidence: "medium",
		},
		CompiledAt: time.Now().UTC(),
	}
	subgraph := model.ContentSubgraph{
		ID:               "web:graph-first-mainline",
		ArticleID:        "web:graph-first-mainline",
		SourcePlatform:   "web",
		SourceExternalID: "graph-first-mainline",
		Nodes: []model.ContentNode{
			{ID: "n1", RawText: "AI资本开支激增", GraphRole: model.GraphRoleDriver, IsPrimary: true},
			{ID: "n2", RawText: "AI叙事压倒宏观冲击", GraphRole: model.GraphRoleIntermediate, IsPrimary: true},
			{ID: "n3", RawText: "纳斯达克上涨", GraphRole: model.GraphRoleTarget, IsPrimary: true},
			{ID: "n4", RawText: "鲍威尔留任", GraphRole: model.GraphRoleDriver, IsPrimary: false},
			{ID: "n5", RawText: "FOMC变数", GraphRole: model.GraphRoleTarget, IsPrimary: false},
		},
		Edges: []model.ContentEdge{
			{ID: "n1->n2:drives", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true},
			{ID: "n2->n3:drives", From: "n2", To: "n3", Type: model.EdgeTypeDrives, IsPrimary: true},
			{ID: "n4->n5:drives", From: "n4", To: "n5", Type: model.EdgeTypeDrives, IsPrimary: false},
		},
	}

	out := formatCompileCard(buildCompileCardProjection(record, &subgraph))
	if !strings.Contains(out, "Mainline") || !strings.Contains(out, "Path: AI资本开支激增 -> AI叙事压倒宏观冲击 -> 纳斯达克上涨") {
		t.Fatalf("stdout = %q, want graph-first primary path in mainline", out)
	}
	if strings.Contains(out, "Side logic\n- AI资本开支激增 -> AI叙事压倒宏观冲击 -> 纳斯达克上涨") {
		t.Fatalf("stdout = %q, did not want primary graph-first path repeated as side logic", out)
	}
}

func TestFormatCompileCardCondensesLargeSemanticInventoryIntoKeyPoints(t *testing.T) {
	units := make([]c.SemanticUnit, 0, 16)
	for i := 0; i < 16; i++ {
		units = append(units, c.SemanticUnit{
			ID:          "semantic-" + string(rune('a'+i)),
			Speaker:     "Greg Abel",
			SpeakerRole: "primary",
			Subject:     "topic " + string(rune('a'+i)),
			Force:       "explain",
			Claim:       "claim " + string(rune('a'+i)),
			Salience:    0.9 - float64(i)/100,
		})
	}
	record := c.Record{
		UnitID:     "youtube:large-inventory",
		Source:     "youtube",
		ExternalID: "large-inventory",
		Model:      varixllm.Qwen36PlusModel,
		Output: c.Output{
			Summary:       "一句话总结",
			SemanticUnits: units,
			Confidence:    "medium",
		},
		CompiledAt: time.Now().UTC(),
	}

	out := formatCompileCard(buildCompileCardProjection(record, nil))
	if !strings.Contains(out, "Key points\n- topic a: claim a") {
		t.Fatalf("stdout = %q, want key points summary for large semantic inventory", out)
	}
	if strings.Contains(out, "Greg Abel / topic a / explain") {
		t.Fatalf("stdout = %q, did not want verbose speaker claim dump for large semantic inventory", out)
	}
	if strings.Contains(out, "topic p: claim p") {
		t.Fatalf("stdout = %q, want key points capped before lower-ranked inventory tail", out)
	}
	if !strings.Contains(out, "Speaker claims\n- 16 total claims stored in the compile output") {
		t.Fatalf("stdout = %q, want inventory note instead of full speaker claim dump", out)
	}
}

func TestFormatCompileCardUsesBriefBeforeSalienceInventory(t *testing.T) {
	record := c.Record{
		UnitID:     "youtube:brief",
		Source:     "youtube",
		ExternalID: "brief",
		Model:      varixllm.Qwen36PlusModel,
		Output: c.Output{
			Summary: "一句话总结",
			Brief: []c.BriefItem{{
				Category: "portfolio",
				Kind:     "list",
				Claim:    "Apple、美运、可口可乐和美国银行仍是核心持仓。",
				Entities: []string{"Apple", "American Express", "Coca-Cola", "Bank of America"},
			}, {
				Category: "culture",
				Kind:     "boundary",
				Claim:    "伯克希尔文化和价值观在交接后保持不变。",
			}},
			SemanticUnits: []c.SemanticUnit{{
				ID:       "semantic-ai",
				Subject:  "AI应用治理",
				Claim:    "AI必须保留人工介入。",
				Salience: 0.99,
			}},
			Confidence: "medium",
		},
		CompiledAt: time.Now().UTC(),
	}

	out := formatCompileCard(buildCompileCardProjection(record, nil))
	if !strings.Contains(out, "Apple, American Express, Coca-Cola, Bank of America: Apple、美运、可口可乐和美国银行仍是核心持仓") {
		t.Fatalf("stdout = %q, want brief portfolio key point", out)
	}
	if !strings.Contains(out, "culture: 伯克希尔文化和价值观在交接后保持不变") {
		t.Fatalf("stdout = %q, want brief culture key point", out)
	}
	if strings.Contains(out, "AI应用治理: AI必须保留人工介入") {
		t.Fatalf("stdout = %q, did not want salience fallback to override brief", out)
	}
}

func TestFormatCompileCardKeepsLegacyLogicChainWhenNoPrimaryMainline(t *testing.T) {
	record := c.Record{
		UnitID:     "web:no-mainline",
		Source:     "web",
		ExternalID: "no-mainline",
		Model:      varixllm.Qwen36PlusModel,
		Output: c.Output{
			Summary: "一句话总结",
			Drivers: []string{"驱动A"},
			Targets: []string{"目标B"},
			TransmissionPaths: []c.TransmissionPath{{
				Driver: "驱动A",
				Steps:  []string{"中间步骤"},
				Target: "目标B",
			}},
			Confidence: "medium",
		},
		CompiledAt: time.Now().UTC(),
	}

	out := formatCompileCard(buildCompileCardProjection(record, nil))
	if strings.Contains(out, "Mainline") {
		t.Fatalf("stdout = %q, did not want mainline without a primary branch", out)
	}
	if !strings.Contains(out, "Logic chain\n- 驱动A -> 中间步骤 -> 目标B") {
		t.Fatalf("stdout = %q, want legacy logic chain fallback", out)
	}
	if strings.Contains(out, "Side logic") {
		t.Fatalf("stdout = %q, did not want side logic without a primary mainline", out)
	}
}
