package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"strings"
	"testing"
	"time"
)

func hasTransmissionPath(paths []TransmissionPath, driver, target string) bool {
	for _, path := range paths {
		if path.Driver == driver && path.Target == target {
			return true
		}
	}
	return false
}

func TestStage5RenderProjectsDriverTargetPathsFromSpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"政策冲击"},{"id":"n2","text":"流动性收缩"},{"id":"n3","text":"美股价格下跌"}]}`},
		{Text: `{"summary":"政策冲击通过流动性收缩压低美股。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "web:spine-render",
		Source:     "web",
		ExternalID: "spine-render",
		Content:    "Policy shock tightens liquidity and lowers equities.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "政策冲击"},
			{ID: "n2", Text: "流动性收缩"},
			{ID: "n3", Text: "美股价格下跌"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Thesis:   "政策冲击压低美股",
			NodeIDs:  []string{"n1", "n2", "n3"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2"},
				{From: "n2", To: "n3"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("rendered output Validate() error = %v", err)
	}
	if out.Graph.Nodes[0].OccurredAt.IsZero() {
		t.Fatalf("OccurredAt is zero for rendered observation node: %#v", out.Graph.Nodes[0])
	}
	if len(out.Drivers) != 1 || out.Drivers[0] != "政策冲击" {
		t.Fatalf("Drivers = %#v, want projected spine source", out.Drivers)
	}
	if len(out.Targets) != 1 || out.Targets[0] != "美股价格下跌" {
		t.Fatalf("Targets = %#v, want projected spine terminal", out.Targets)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want one spine path", out.TransmissionPaths)
	}
}

func TestStage5RenderSurfacesSemanticUnitsInSummary(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"股东提出能力圈问题"},{"id":"n2","text":"Greg Abel 回答 Apple 投资逻辑"}]}`},
		{Text: `{"summary":"伯克希尔会等待市场错配，再快速、大额部署资本。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:semantic-render",
		Source:     "youtube",
		ExternalID: "semantic-render",
		Content:    "Greg Abel answered Apple is not held because it is a technology stock.",
	}, graphState{
		ArticleForm: "shareholder_meeting",
		Nodes: []graphNode{
			{ID: "n1", Text: "股东提出能力圈问题", Role: roleDriver},
			{ID: "n2", Text: "Greg Abel 回答 Apple 投资逻辑", IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
		SemanticUnits: []SemanticUnit{{
			ID:               "semantic-apple",
			Speaker:          "Greg Abel",
			SpeakerRole:      "primary",
			Subject:          "existing portfolio / circle of competence",
			Force:            "answer",
			Claim:            "现有组合由 Warren Buffett 建立，但集中在 Greg Abel 也理解业务和经济前景的公司；Apple 说明能力圈不是行业标签，而是看产品价值、消费者依赖和风险。",
			PromptContext:    "股东询问 Greg Abel 如何管理 Warren Buffett 建立的组合。",
			ImportanceReason: "这是主讲人对投资科技股/能力圈问题的直接回答。",
			SourceQuote:      "not because we view it as a technology stock",
			Salience:         0.93,
			Confidence:       "high",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if out.Summary != "Greg Abel阐明组合管理纪律" || len([]rune(out.Summary)) > 30 {
		t.Fatalf("Summary = %q, want compact semantic summary", out.Summary)
	}
	if len(out.SemanticUnits) != 1 {
		t.Fatalf("SemanticUnits = %#v, want rendered unit", out.SemanticUnits)
	}
}

func TestStage5SummaryRequestIncludesSemanticUnits(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"管理层问题"},{"id":"n2","text":"管理层回答"}]}`},
		{Text: `{"summary":"管理层回答资本配置和AI边界。"}`},
	}}
	_, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:summary-semantic",
		Source:     "youtube",
		ExternalID: "summary-semantic",
		Content:    "management Q&A",
	}, graphState{
		ArticleForm: "shareholder_meeting",
		Nodes: []graphNode{
			{ID: "n1", Text: "管理层问题", Role: roleDriver},
			{ID: "n2", Text: "管理层回答", IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
		SemanticUnits: []SemanticUnit{{
			ID:          "semantic-ai-boundary",
			Speaker:     "Greg Abel",
			SpeakerRole: "primary",
			Subject:     "AI governance",
			Force:       "set_boundary",
			Claim:       "AI 不用于核心定价决策。",
			Salience:    0.9,
			Confidence:  "high",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(rt.requests) < 2 || !providerRequestContains(rt.requests[1], "semantic_units") {
		t.Fatalf("summary request did not include semantic units: %#v", rt.requests)
	}
}

func TestSummarySemanticUnitsPrioritizeReaderInterest(t *testing.T) {
	units := topSemanticUnitsForSummary([]SemanticUnit{
		{
			ID:         "semantic-market",
			Subject:    "保险市场软化与资本涌入",
			Force:      "frame_risk",
			Claim:      "保险市场趋于软化。",
			Salience:   0.99,
			Confidence: "high",
		},
		{
			ID:         "semantic-capital",
			Subject:    "capital allocation",
			Force:      "commit",
			Claim:      "只有在机会足够好时才快速、大额部署资本。",
			Salience:   0.93,
			Confidence: "high",
		},
		{
			ID:         "semantic-portfolio",
			Subject:    "existing portfolio / circle of competence",
			Force:      "answer",
			Claim:      "Greg Abel 解释会如何管理 Warren Buffett 建立的组合。",
			Salience:   0.92,
			Confidence: "high",
		},
		{
			ID:         "semantic-buyback",
			Subject:    "股票回购触发条件",
			Force:      "set_boundary",
			Claim:      "只有股价低于保守估算内在价值时才回购。",
			Salience:   0.91,
			Confidence: "high",
		},
	}, "shareholder_meeting")
	if len(units) < 4 {
		t.Fatalf("topSemanticUnitsForSummary returned %d units, want 4", len(units))
	}
	got := []string{units[0].ID, units[1].ID, units[2].ID}
	want := []string{"semantic-capital", "semantic-portfolio", "semantic-buyback"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("summary unit order = %v, want reader-interest order %v", got, want)
	}
}

func TestReaderInterestSummaryAppendsHumanQuestionBeforeBackground(t *testing.T) {
	summary := prioritizeSemanticSummary("伯克希尔把现金头寸视为资本配置选择权，只有机会足够好时才快速、大额部署资本。", []SemanticUnit{
		{
			ID:       "semantic-market",
			Subject:  "保险市场软化与资本涌入",
			Force:    "frame_risk",
			Claim:    "保险市场趋于软化，获取合理风险溢价面临更大挑战。",
			Salience: 0.99,
		},
		{
			ID:       "semantic-portfolio",
			Subject:  "existing portfolio / circle of competence",
			Force:    "answer",
			Claim:    "Greg Abel 的回答是：现有组合由 Warren Buffett 建立，但集中在他也理解业务和经济前景的公司，所以他对组合很舒服；之后会持续评估业务演化和新风险。Apple 是例子，说明伯克希尔判断能力圈不按“科技股”标签，而是看产品价值、消费者依赖、耐久性和风险。",
			Salience: 0.93,
		},
		{
			ID:       "semantic-buyback",
			Subject:  "股票回购触发条件",
			Force:    "set_boundary",
			Claim:    "伯克希尔只有在股价低于保守估算内在价值时才回购。",
			Salience: 0.91,
		},
	}, "shareholder_meeting")
	if !strings.Contains(summary, "现有组合由 Warren Buffett 建立") {
		t.Fatalf("Summary = %q, want portfolio/circle answer included", summary)
	}
	if strings.Contains(summary, "Apple 是例子") {
		t.Fatalf("Summary = %q, want summary-level claim rather than full speaker-card detail", summary)
	}
	if strings.Contains(summary, "保险市场趋于软化") {
		t.Fatalf("Summary = %q, should not spend reader-interest summary slot on background market risk", summary)
	}
}

func TestCompactSummaryKeepsMeetingSummaryUnderThirtyCharacters(t *testing.T) {
	got := compactSummaryForDisplay(
		"伯克希尔的现金头寸为其在各业务板块间灵活配置资本提供了选择权；Greg Abel 表示现有组合由 Warren Buffett 建立，但集中在他也理解业务和经济前景的公司。",
		"shareholder_meeting",
		[]Declaration{{
			Speaker: "Greg Abel",
			Kind:    "capital_allocation_rule",
			Topic:   "capital_allocation",
		}},
		[]SemanticUnit{{
			Speaker: "Greg Abel",
			Subject: "existing portfolio / circle of competence",
			Force:   "answer",
			Claim:   "Greg Abel 解释如何管理 Warren Buffett 建立的组合。",
		}},
	)
	if len([]rune(got)) > 30 {
		t.Fatalf("compact summary = %q (%d runes), want <= 30", got, len([]rune(got)))
	}
	if got != "Abel延续巴菲特式资本纪律" {
		t.Fatalf("compact summary = %q", got)
	}
}

func TestTitleFromUnitsDerivesContinuityWithoutBuffettSpecialCase(t *testing.T) {
	got := compactSummaryForDisplay(
		strings.Repeat("长摘要", 20),
		"shareholder_meeting",
		[]Declaration{{
			Speaker: "李明",
			Kind:    "capital_allocation_rule",
			Topic:   "capital_allocation",
		}},
		[]SemanticUnit{{
			Speaker: "李明",
			Subject: "existing portfolio / circle of competence",
			Force:   "answer",
			Claim:   "现有组合由王强建立，但李明也理解这些公司的业务和经济前景。",
		}},
	)
	if got != "李明延续王强式资本纪律" {
		t.Fatalf("compact summary = %q", got)
	}
}

func TestCompactSummaryJoinsThreeReaderTopicsNaturally(t *testing.T) {
	got := compactSummaryForDisplay(
		strings.Repeat("长摘要", 20),
		"shareholder_meeting",
		[]Declaration{{Speaker: "Greg Abel", Kind: "capital_allocation_rule", Topic: "capital_allocation"}},
		[]SemanticUnit{
			{Speaker: "Greg Abel", Subject: "existing portfolio / circle of competence", Force: "answer", Claim: "组合管理。"},
			{Speaker: "Greg Abel", Subject: "股票回购触发条件", Force: "set_boundary", Claim: "回购纪律。"},
		},
	)
	if got != "Greg Abel阐明资本配置、组合管理与回购纪律" {
		t.Fatalf("compact summary = %q", got)
	}
	if len([]rune(got)) > 30 {
		t.Fatalf("compact summary = %q (%d runes), want <= 30", got, len([]rune(got)))
	}
}

func providerRequestContains(req llm.ProviderRequest, needle string) bool {
	if strings.Contains(req.User, needle) || strings.Contains(req.System, needle) {
		return true
	}
	for _, part := range req.UserParts {
		if strings.Contains(part.Text, needle) {
			return true
		}
	}
	return false
}

func TestStage5RenderBuildsCapitalAllocationDeclarationFromSpine(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n_cash","text":"伯克希尔持有约3800亿美元现金和短债"},{"id":"n_rule","text":"伯克希尔会等待市场错配"},{"id":"n_action","text":"出现机会时会快速且果断行动"},{"id":"n_scale","text":"会投入大量资本"},{"id":"n_boundary","text":"不会仅因现金规模大而被迫投资"}]}`},
		{Text: `{"summary":"伯克希尔会保留大量现金和短债，等待市场错配后快速、果断、大额部署资本。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:berkshire-declaration",
		Source:     "youtube",
		ExternalID: "berkshire-declaration",
		Content:    "Greg Abel said Berkshire has large cash reserves and will act quickly with significant capital when dislocations appear.",
	}, graphState{
		ArticleForm: "management_qa",
		Nodes: []graphNode{
			{ID: "n_cash", Text: "Berkshire holds about $380B in cash and Treasury bills", DiscourseRole: "evidence", SourceQuote: "our cash in US Treasury bills net is 380 billion"},
			{ID: "n_rule", Text: "Berkshire will wait for market dislocations", DiscourseRole: "capital_allocation_rule", SourceQuote: "there will be dislocations in markets"},
			{ID: "n_action", Text: "Berkshire will act quickly and decisively when opportunities appear", DiscourseRole: "action", SourceQuote: "we'll act decisively both quickly"},
			{ID: "n_scale", Text: "Berkshire can deploy significant capital", DiscourseRole: "scale", SourceQuote: "with significant capital"},
			{ID: "n_boundary", Text: "Berkshire will not force deployment just because cash is large", DiscourseRole: "constraint", SourceQuote: "we don't have to swing at every pitch"},
		},
		Edges: []graphEdge{
			{From: "n_cash", To: "n_rule"},
			{From: "n_rule", To: "n_action"},
			{From: "n_action", To: "n_scale"},
			{From: "n_rule", To: "n_boundary"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "capital_allocation_rule",
			Thesis:   "Greg Abel explains how Berkshire will deploy cash",
			NodeIDs:  []string{"n_cash", "n_rule", "n_action", "n_scale", "n_boundary"},
			Edges: []PreviewEdge{
				{From: "n_cash", To: "n_rule"},
				{From: "n_rule", To: "n_action"},
				{From: "n_action", To: "n_scale"},
				{From: "n_rule", To: "n_boundary"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("rendered output Validate() error = %v", err)
	}
	if len(out.Declarations) != 1 {
		t.Fatalf("Declarations = %#v, want one declaration", out.Declarations)
	}
	got := out.Declarations[0]
	if got.Kind != "capital_allocation_rule" {
		t.Fatalf("Declaration.Kind = %q, want capital_allocation_rule", got.Kind)
	}
	if got.Topic != "capital_allocation" {
		t.Fatalf("Declaration.Topic = %q, want capital_allocation", got.Topic)
	}
	if got.Statement != "伯克希尔会等待市场错配" {
		t.Fatalf("Declaration.Statement = %q", got.Statement)
	}
	if !containsString(got.Actions, "出现机会时会快速且果断行动") {
		t.Fatalf("Declaration.Actions = %#v, want action", got.Actions)
	}
	if got.Scale != "会投入大量资本" {
		t.Fatalf("Declaration.Scale = %q", got.Scale)
	}
	if !containsString(got.Constraints, "不会仅因现金规模大而被迫投资") {
		t.Fatalf("Declaration.Constraints = %#v, want non-forced deployment boundary", got.Constraints)
	}
	if !containsString(got.Evidence, "伯克希尔持有约3800亿美元现金和短债") {
		t.Fatalf("Declaration.Evidence = %#v, want cash reserve support", got.Evidence)
	}
	if len(out.Branches) != 1 || len(out.Branches[0].Declarations) != 1 {
		t.Fatalf("Branches = %#v, want branch declaration", out.Branches)
	}
	if len(out.TransmissionPaths) != 0 {
		t.Fatalf("TransmissionPaths = %#v, declaration spine should not be rendered as causal path", out.TransmissionPaths)
	}
}

func TestStage5RenderDerivesCapitalAllocationDeclarationSlotsFromQuote(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n_cash","text":"伯克希尔现金和短债净额为3800亿美元"},{"id":"n_rule","text":"伯克希尔的资本配置理念强调耐心与纪律，仅在具备显著投资价值时果断出手"}]}`},
		{Text: `{"summary":"伯克希尔会保持资本配置耐心，只在市场错配时大额出手。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:berkshire-quote-slots",
		Source:     "youtube",
		ExternalID: "berkshire-quote-slots",
		Content:    "Greg Abel discussed capital allocation.",
	}, graphState{
		ArticleForm: "management_qa",
		Nodes: []graphNode{
			{ID: "n_cash", Text: "Berkshire's cash and Treasury bills net to $380B", DiscourseRole: "evidence", SourceQuote: "our cash in U US Treasury bills net is 380 billion"},
			{ID: "n_rule", Text: "Berkshire's capital allocation emphasizes patience and discipline", DiscourseRole: "capital_allocation_rule", SourceQuote: "patience and the discipline around capital allocation... there will be dislocations in markets... act decisively both quickly and with significant capital"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "capital_allocation_rule",
			Thesis:   "Berkshire explains its cash deployment rule",
			NodeIDs:  []string{"n_cash", "n_rule"},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Declarations) != 1 {
		t.Fatalf("Declarations = %#v, want one declaration", out.Declarations)
	}
	got := out.Declarations[0]
	if !containsString(got.Conditions, "市场出现错配") {
		t.Fatalf("Conditions = %#v, want market dislocation condition", got.Conditions)
	}
	if !containsString(got.Conditions, "机会具备显著投资价值") {
		t.Fatalf("Conditions = %#v, want value proposition condition", got.Conditions)
	}
	if !containsString(got.Actions, "快速且果断行动") {
		t.Fatalf("Actions = %#v, want decisive action", got.Actions)
	}
	if got.Scale != "投入大量资本" {
		t.Fatalf("Scale = %q, want significant capital", got.Scale)
	}
	if !containsString(got.Constraints, "保持资本配置耐心与纪律") {
		t.Fatalf("Constraints = %#v, want patience/discipline boundary", got.Constraints)
	}
	if out.Summary != "Greg Abel阐明资本配置纪律" {
		t.Fatalf("Summary = %q, want declaration-priority summary", out.Summary)
	}
}

func TestStage5RenderRescuesCapitalAllocationDeclarationFromManagementTranscript(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"GEICO上调保费"},{"id":"n2","text":"客户留存承压"}]}`},
		{Text: `{"summary":"GEICO上调保费使客户留存承压。"}`},
	}}
	transcript := `Greg Abel: our greatest strengths at Berkshire is patience and being disciplined when it comes to allocating our capital.
There will be opportunities that come over time. We want to know it meets our principles.
You will feel there is a strong value proposition with an opportunity. If it presents itself, we'll be prepared to act decisively both quickly and with significant capital.
If you look at our cash and US Treasury bills, net is 380 billion.`
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:berkshire-source-rescue",
		Source:     "youtube",
		ExternalID: "berkshire-source-rescue",
		Content:    transcript,
	}, graphState{
		ArticleForm: "management_qa",
		Nodes: []graphNode{
			{ID: "n1", Text: "GEICO increased premiums"},
			{ID: "n2", Text: "customer retention is under pressure", IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
		Spines: []PreviewSpine{{
			ID:      "s1",
			Policy:  "causal_mechanism",
			Thesis:  "GEICO premiums pressure retention",
			NodeIDs: []string{"n1", "n2"},
			Edges:   []PreviewEdge{{From: "n1", To: "n2"}},
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Declarations) != 1 {
		t.Fatalf("Declarations = %#v, want rescued capital allocation declaration", out.Declarations)
	}
	got := out.Declarations[0]
	if got.Kind != "capital_allocation_rule" || got.Topic != "capital_allocation" {
		t.Fatalf("Declaration = %#v, want capital allocation rule", got)
	}
	if !containsString(got.Conditions, "机会具备显著投资价值") {
		t.Fatalf("Conditions = %#v, want value proposition condition", got.Conditions)
	}
	if !containsString(got.Actions, "快速且果断行动") {
		t.Fatalf("Actions = %#v, want decisive action", got.Actions)
	}
	if got.Scale != "投入大量资本" {
		t.Fatalf("Scale = %q, want significant capital", got.Scale)
	}
	if !containsString(got.Constraints, "保持资本配置耐心与纪律") {
		t.Fatalf("Constraints = %#v, want patience/discipline boundary", got.Constraints)
	}
	if strings.Contains(got.SourceQuote, ":34:") || strings.Count(got.SourceQuote, "patience and being disciplined") > 1 {
		t.Fatalf("SourceQuote = %q, want compact de-duplicated quote fragments", got.SourceQuote)
	}
	if out.Summary != "Greg Abel阐明资本配置纪律" {
		t.Fatalf("Summary = %q, want rescued declaration summary", out.Summary)
	}
}

func TestStage5RenderMergesSourceCapitalAllocationSlotsIntoExistingDeclaration(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n_rule","text":"伯克希尔充裕的现金储备支持其在各业务板块间灵活配置资本"}]}`},
		{Text: `{"summary":"伯克希尔有大量现金用于资本配置。"}`},
	}}
	transcript := `Greg Abel: our greatest strengths at Berkshire is patience and being disciplined when it comes to allocating our capital.
We are not anxious to deploy capital into subpar opportunities. We want to know it meets our principles.
If there is a strong value proposition and it presents itself, we'll act decisively both quickly and with significant capital.
Our cash and US Treasury bills net is 380 billion.`
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:berkshire-existing-declaration",
		Source:     "youtube",
		ExternalID: "berkshire-existing-declaration",
		Content:    transcript,
	}, graphState{
		ArticleForm: "management_qa",
		Nodes: []graphNode{
			{ID: "n_rule", Text: "Berkshire's cash creates optionality to deploy capital across groups", DiscourseRole: "capital_allocation_rule", SourceQuote: "creates the opportunity to deploy it across these different groups / we'll act decisively both quickly and with significant capital"},
		},
		Spines: []PreviewSpine{{
			ID:      "s1",
			Policy:  "capital_allocation_rule",
			Thesis:  "Berkshire can deploy cash across groups",
			NodeIDs: []string{"n_rule"},
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Declarations) != 1 {
		t.Fatalf("Declarations = %#v, want one merged declaration", out.Declarations)
	}
	got := out.Declarations[0]
	if got.ID != "s1" {
		t.Fatalf("Declaration.ID = %q, want existing spine id", got.ID)
	}
	if !containsString(got.Conditions, "机会具备显著投资价值") {
		t.Fatalf("Conditions = %#v, want source-rescued condition", got.Conditions)
	}
	if !containsString(got.Constraints, "保持资本配置耐心与纪律") {
		t.Fatalf("Constraints = %#v, want source-rescued discipline boundary", got.Constraints)
	}
	if !containsString(got.Evidence, "伯克希尔现金及美国短债净额约为3800亿美元") {
		t.Fatalf("Evidence = %#v, want source-rescued cash evidence", got.Evidence)
	}
	if out.Summary != "Greg Abel阐明资本配置纪律" {
		t.Fatalf("Summary = %q, want merged declaration summary", out.Summary)
	}
}

func TestDeclarationCoverageGateAddsMissingCapitalAllocationSlotsBeforeRender(t *testing.T) {
	transcript := `Greg Abel: our greatest strengths at Berkshire is patience and being disciplined when it comes to allocating our capital.
We are not anxious to deploy capital into subpar opportunities. We want to know it meets our principles.
If there is a strong value proposition and it presents itself, we'll act decisively both quickly and with significant capital.
Our cash and US Treasury bills net is 380 billion.`
	state := graphState{
		ArticleForm: "management_qa",
		Nodes: []graphNode{
			{ID: "n_rule", Text: "Berkshire's cash creates optionality to deploy capital across groups", DiscourseRole: "capital_allocation_rule", SourceQuote: "creates the opportunity to deploy it across these different groups"},
		},
		Spines: []PreviewSpine{{
			ID:      "s1",
			Policy:  "capital_allocation_rule",
			Thesis:  "Berkshire can deploy cash across groups",
			NodeIDs: []string{"n_rule"},
		}},
	}
	got := applyDeclarationCoverageGate(Bundle{
		UnitID:     "youtube:berkshire-coverage-gate",
		Source:     "youtube",
		ExternalID: "berkshire-coverage-gate",
		Content:    transcript,
	}, state)
	roles := map[string]graphNode{}
	for _, node := range got.Nodes {
		roles[normalizeDiscourseRole(node.DiscourseRole)] = node
	}
	for _, role := range []string{"condition", "action", "scale", "constraint", "evidence"} {
		if strings.TrimSpace(roles[role].Text) == "" {
			t.Fatalf("coverage gate nodes = %#v, want role %q", got.Nodes, role)
		}
	}
	if len(got.Spines) != 1 {
		t.Fatalf("Spines = %#v, want existing spine only", got.Spines)
	}
	for _, node := range roles {
		if !containsString(got.Spines[0].NodeIDs, node.ID) {
			t.Fatalf("Spine node IDs = %#v, want coverage node %q attached", got.Spines[0].NodeIDs, node.ID)
		}
	}
}

func TestStage5RenderDedupesDuplicateStateNodesBeforeOutput(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"油价冲击"},{"id":"n2","text":"商品价格上涨"}]}`},
		{Text: `{"summary":"油价冲击推高商品价格。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:duplicate-render",
		Source:     "youtube",
		ExternalID: "duplicate-render",
		Content:    "Oil shock raises goods prices.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Oil shock", Role: roleDriver},
			{ID: "n2", Text: "Goods prices rise", IsTarget: true},
			{ID: "n2", Text: "Goods prices rise", IsTarget: true},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n1", To: "n2"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Thesis:   "油价冲击推高商品价格",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("rendered output Validate() error = %v", err)
	}
	if len(out.Graph.Nodes) != 2 {
		t.Fatalf("graph nodes = %#v, want duplicate node collapsed", out.Graph.Nodes)
	}
}

func TestStage5RenderFallsBackForLowStructureContent(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"作者感谢同事并宣布离开 IIF"}]}`},
		{Text: `{"summary":"作者感谢同事并宣布离开 IIF。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "twitter:low-structure",
		Source:     "twitter",
		ExternalID: "low-structure",
		Content:    "Today is my last day at the IIF. I'd like to thank my friends and colleagues.",
		PostedAt:   time.Date(2024, 1, 26, 12, 0, 0, 0, time.UTC),
	}, graphState{
		Nodes: []graphNode{{ID: "n1", Text: "作者感谢同事并宣布离开 IIF"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("low-structure fallback Validate() error = %v", err)
	}
	if out.Confidence != "low" {
		t.Fatalf("Confidence = %q, want low", out.Confidence)
	}
	if len(out.Graph.Nodes) != 2 || len(out.Graph.Edges) != 1 {
		t.Fatalf("Graph = %#v, want fallback graph with two nodes and one edge", out.Graph)
	}
	if !containsString(out.Details.Caveats, "low-structure content fallback") {
		t.Fatalf("Caveats = %#v, want low-structure fallback marker", out.Details.Caveats)
	}
}

func TestStage5RenderIncludesAttachedOffGraphPremisesInPathSteps(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"AI基础设施支出规模庞大"},{"id":"n2","text":"小型软件公司跑输现金充裕平台"},{"id":"o1","text":"信用利差走阔"}]}`},
		{Text: `{"summary":"AI支出与信用利差共同影响小型软件公司。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "web:offgraph-premise-render",
		Source:     "web",
		ExternalID: "offgraph-premise-render",
		Content:    "If credit spreads widen while AI capex accelerates, smaller software companies may underperform.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "AI capex accelerates"},
			{ID: "n2", Text: "Smaller software companies underperform", IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
		OffGraph: []offGraphItem{{
			ID:          "o1",
			Text:        "Credit spreads widen",
			Role:        "inference",
			AttachesTo:  "n2",
			SourceQuote: "credit spreads widen",
		}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "forecast_inference",
			Thesis:   "AI capex and credit spreads imply software underperformance",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "inference"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want one path", out.TransmissionPaths)
	}
	if !containsString(out.TransmissionPaths[0].Steps, "信用利差走阔") {
		t.Fatalf("TransmissionPath steps = %#v, want attached off-graph premise", out.TransmissionPaths[0].Steps)
	}
	if len(out.Branches) != 1 || len(out.Branches[0].TransmissionPaths) != 1 || !containsString(out.Branches[0].TransmissionPaths[0].Steps, "信用利差走阔") {
		t.Fatalf("Branch paths = %#v, want attached off-graph premise", out.Branches)
	}
}

func TestStage5RenderBuildsBranchLocalRolesFromSpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"战争前兆指标"},{"id":"n2","text":"长期世界大战"},{"id":"n3","text":"阵营对立"},{"id":"n4","text":"中俄相对赢家"}]}`},
		{Text: `{"summary":"战争前兆指标指向长期世界大战，阵营对立使中俄成为相对赢家。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "twitter:branch-render",
		Source:  "twitter",
		Content: "Long-form forecast with two branches.",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "战争前兆指标"},
			{ID: "n2", Text: "长期世界大战"},
			{ID: "n3", Text: "阵营对立"},
			{ID: "n4", Text: "中俄相对赢家"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Policy:   "forecast_inference",
				Thesis:   "战争前兆指标显示长期世界大战",
				NodeIDs:  []string{"n1", "n2"},
				Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "inference"}},
				Scope:    "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Policy:   "geopolitical_trade_realignment",
				Thesis:   "阵营对立使中俄相对受益",
				NodeIDs:  []string{"n3", "n4"},
				Edges:    []PreviewEdge{{From: "n3", To: "n4", Kind: "causal"}},
				Scope:    "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Branches) != 2 {
		t.Fatalf("Branches = %#v, want two branch-local views", out.Branches)
	}
	if len(out.Branches[0].Drivers) != 1 || out.Branches[0].Drivers[0] != "战争前兆指标" {
		t.Fatalf("primary branch drivers = %#v, want branch-local driver", out.Branches[0].Drivers)
	}
	if len(out.Branches[0].Targets) != 1 || out.Branches[0].Targets[0] != "长期世界大战" {
		t.Fatalf("primary branch targets = %#v, want branch-local target", out.Branches[0].Targets)
	}
	if len(out.Branches[0].TransmissionPaths) != 1 || out.Branches[0].TransmissionPaths[0].Driver != "战争前兆指标" || out.Branches[0].TransmissionPaths[0].Target != "长期世界大战" {
		t.Fatalf("primary branch paths = %#v, want local transmission path", out.Branches[0].TransmissionPaths)
	}
	if len(out.Branches[1].Drivers) != 1 || out.Branches[1].Drivers[0] != "阵营对立" {
		t.Fatalf("second branch drivers = %#v, want branch-local driver", out.Branches[1].Drivers)
	}
	if len(out.Branches[1].Targets) != 1 || out.Branches[1].Targets[0] != "中俄相对赢家" {
		t.Fatalf("second branch targets = %#v, want branch-local target", out.Branches[1].Targets)
	}
}

func TestStage5RenderSeparatesCommonAnchorFromBranchDrivers(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"长期世界大战"},{"id":"n2","text":"秩序转向强权逻辑"},{"id":"n3","text":"冲突增加"},{"id":"n4","text":"美国过度扩张"},{"id":"n5","text":"世界秩序被重塑"}]}`},
		{Text: `{"summary":"长期世界大战通过不同分支影响秩序与美国脆弱性。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "twitter:anchor-render",
		Source:  "twitter",
		Content: "A macro frame supports several local branches.",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "长期世界大战"},
			{ID: "n2", Text: "秩序转向强权逻辑"},
			{ID: "n3", Text: "冲突增加"},
			{ID: "n4", Text: "美国过度扩张"},
			{ID: "n5", Text: "世界秩序被重塑"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Policy:   "causal_mechanism",
				Thesis:   "长期世界大战推动秩序转向强权并增加冲突",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "causal"}, {From: "n2", To: "n3", Kind: "causal"}},
				Scope:    "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Policy:   "causal_mechanism",
				Thesis:   "长期世界大战暴露美国过度扩张并重塑秩序",
				NodeIDs:  []string{"n1", "n4", "n5"},
				Edges:    []PreviewEdge{{From: "n1", To: "n4", Kind: "causal"}, {From: "n4", To: "n5", Kind: "causal"}},
				Scope:    "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Branches) != 2 {
		t.Fatalf("Branches = %#v, want two branches", out.Branches)
	}
	if len(out.Branches[0].Anchors) != 1 || out.Branches[0].Anchors[0] != "长期世界大战" {
		t.Fatalf("first branch anchors = %#v, want shared macro frame", out.Branches[0].Anchors)
	}
	if len(out.Branches[0].BranchDrivers) != 1 || out.Branches[0].BranchDrivers[0] != "秩序转向强权逻辑" {
		t.Fatalf("first branch drivers = %#v, want local mechanism", out.Branches[0].BranchDrivers)
	}
	if len(out.Branches[1].Anchors) != 1 || out.Branches[1].Anchors[0] != "长期世界大战" {
		t.Fatalf("second branch anchors = %#v, want shared macro frame", out.Branches[1].Anchors)
	}
	if len(out.Branches[1].BranchDrivers) != 1 || out.Branches[1].BranchDrivers[0] != "美国过度扩张" {
		t.Fatalf("second branch drivers = %#v, want local mechanism", out.Branches[1].BranchDrivers)
	}
}

func TestStage5RenderProjectsSatiricalAnalogyFromRealMechanism(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"村长与新富设计幸运游戏"},{"id":"n2","text":"牌照化金融游戏把资金转移包装成公平规则"},{"id":"n3","text":"多数后续参与者承担机会成本与管理费"}]}`},
		{Text: `{"summary":"牌照化金融游戏把资金转移包装成公平规则，导致多数后续参与者承担机会成本与管理费。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-render",
		Source:  "bilibili",
		Content: "A village game satirizes institutional finance.",
	}, graphState{
		ArticleForm: "institutional_satire",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "牌照化金融游戏把资金转移包装成公平规则", DiscourseRole: "thesis"},
			{ID: "n3", Text: "多数后续参与者承担机会成本与管理费", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2", Kind: "illustration"},
			{From: "n2", To: "n3", Kind: "causal"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏讽刺牌照化金融规则转嫁成本",
			NodeIDs:  []string{"n1", "n2", "n3"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "illustration"},
				{From: "n2", To: "n3", Kind: "causal"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "村长与新富设计幸运游戏") {
		t.Fatalf("Drivers = %#v, want satire vehicle kept out of display drivers", out.Drivers)
	}
	if !containsString(out.Drivers, "牌照化金融游戏把资金转移包装成公平规则") {
		t.Fatalf("Drivers = %#v, want real institutional mechanism as display driver", out.Drivers)
	}
	if !containsString(out.Targets, "多数后续参与者承担机会成本与管理费") {
		t.Fatalf("Targets = %#v, want real-world transferred cost as display target", out.Targets)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "牌照化金融游戏把资金转移包装成公平规则", "多数后续参与者承担机会成本与管理费") {
		t.Fatalf("TransmissionPaths = %#v, want real mechanism path instead of allegory-source path", out.TransmissionPaths)
	}
	if !containsString(out.EvidenceNodes, "村长与新富设计幸运游戏") {
		t.Fatalf("EvidenceNodes = %#v, want satire vehicle displayed as evidence/illustration", out.EvidenceNodes)
	}
}

func TestStage5RenderKeepsAnalogySourceOutOfSatiricalDriversWithoutIllustrationKind(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"村长与新富设计幸运游戏"},{"id":"n2","text":"游戏本质不公平但可包装成公平"},{"id":"n3","text":"多数后续参与者承担机会成本与管理费"}]}`},
		{Text: `{"summary":"游戏本质不公平但可包装成公平，导致多数后续参与者承担机会成本与管理费。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-render-causal-edge",
		Source:  "bilibili",
		Content: "A village game satirizes institutional finance.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "游戏本质不公平但可包装成公平", DiscourseRole: "satire_target"},
			{ID: "n3", Text: "多数后续参与者承担机会成本与管理费", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2", Kind: "causal"},
			{From: "n2", To: "n3", Kind: "causal"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏讽刺规则包装后的成本转嫁",
			NodeIDs:  []string{"n1", "n2", "n3"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "causal"},
				{From: "n2", To: "n3", Kind: "causal"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "村长与新富设计幸运游戏") {
		t.Fatalf("Drivers = %#v, want analogy node kept out of display drivers", out.Drivers)
	}
	if !containsString(out.Drivers, "游戏本质不公平但可包装成公平") {
		t.Fatalf("Drivers = %#v, want satire target as display driver", out.Drivers)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "游戏本质不公平但可包装成公平", "多数后续参与者承担机会成本与管理费") {
		t.Fatalf("TransmissionPaths = %#v, want satire target path", out.TransmissionPaths)
	}
}

func TestStage5RenderDropsCyclicSpineProjectionPath(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"不公平游戏"},{"id":"n2","text":"包装资产"},{"id":"n3","text":"安全感下降"},{"id":"n4","text":"财务自由变远"}]}`},
		{Text: `{"summary":"不公平游戏通过包装资产和安全感下降拉长财务自由进程。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:cyclic-spines",
		Source:  "bilibili",
		Content: "Satire plus branches.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "不公平游戏", DiscourseRole: "thesis"},
			{ID: "n2", Text: "包装资产", DiscourseRole: "satire_target"},
			{ID: "n3", Text: "安全感下降", DiscourseRole: "mechanism"},
			{ID: "n4", Text: "财务自由变远", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n3", To: "n4"},
			{From: "n4", To: "n1"},
		},
		Spines: []PreviewSpine{
			{ID: "s1", Level: "primary", Priority: 1, Policy: "satirical_analogy", Thesis: "不公平游戏映射包装资产", NodeIDs: []string{"n1", "n2"}, Edges: []PreviewEdge{{From: "n1", To: "n2"}}, Scope: "article"},
			{ID: "s2", Level: "branch", Priority: 2, Policy: "causal_mechanism", Thesis: "包装资产降低安全感", NodeIDs: []string{"n2", "n3"}, Edges: []PreviewEdge{{From: "n2", To: "n3"}}, Scope: "section"},
			{ID: "s3", Level: "branch", Priority: 3, Policy: "investment_implication", Thesis: "安全感下降拉长自由进程", NodeIDs: []string{"n3", "n4"}, Edges: []PreviewEdge{{From: "n3", To: "n4"}}, Scope: "section"},
			{ID: "s4", Level: "branch", Priority: 4, Policy: "investment_implication", Thesis: "错误回环", NodeIDs: []string{"n4", "n1"}, Edges: []PreviewEdge{{From: "n4", To: "n1"}}, Scope: "section"},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if hasTransmissionPath(out.TransmissionPaths, "财务自由变远", "不公平游戏") {
		t.Fatalf("TransmissionPaths = %#v, want cycle-closing path dropped", out.TransmissionPaths)
	}
	if containsString(out.Targets, "不公平游戏") {
		t.Fatalf("Targets = %#v, want driver not reintroduced as target by cycle fallback", out.Targets)
	}
	if !containsString(out.Targets, "财务自由变远") {
		t.Fatalf("Targets = %#v, want final acyclic terminal retained", out.Targets)
	}
}

func TestStage5RenderDropsCycleClosingPathThroughIntermediateStep(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"初始驱动"},{"id":"n2","text":"中间机制"},{"id":"n3","text":"最终结果"}]}`},
		{Text: `{"summary":"初始驱动经由中间机制导致最终结果。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:multi-step-cycle",
		Source:  "web",
		Content: "A drives B, B drives C, with a bad B-to-A branch.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "初始驱动", Role: roleDriver},
			{ID: "n2", Text: "中间机制", Role: roleTransmission},
			{ID: "n3", Text: "最终结果", Role: roleTransmission, IsTarget: true},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Policy:   "causal_mechanism",
				Thesis:   "初始驱动经由中间机制导致最终结果",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2", Kind: "causal"},
					{From: "n2", To: "n3", Kind: "causal"},
				},
				Scope: "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Policy:   "causal_mechanism",
				Thesis:   "错误回指",
				NodeIDs:  []string{"n2", "n1"},
				Edges:    []PreviewEdge{{From: "n2", To: "n1", Kind: "causal"}},
				Scope:    "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "初始驱动", "最终结果") {
		t.Fatalf("TransmissionPaths = %#v, want multi-step primary path retained", out.TransmissionPaths)
	}
	if hasTransmissionPath(out.TransmissionPaths, "中间机制", "初始驱动") {
		t.Fatalf("TransmissionPaths = %#v, want back-edge through intermediate step dropped", out.TransmissionPaths)
	}
	if containsString(out.Targets, "初始驱动") {
		t.Fatalf("Targets = %#v, want driver not reintroduced as target through back-edge", out.Targets)
	}
}

func TestStage5RenderDropsMultiStepPathClosingPriorEndpointCycle(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"初始驱动"},{"id":"n2","text":"中间机制"},{"id":"n3","text":"最终结果"}]}`},
		{Text: `{"summary":"错误回指不应再允许初始驱动形成回环路径。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:prior-endpoint-cycle",
		Source:  "web",
		Content: "C points back to A before a later A-B-C projection.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "初始驱动", Role: roleDriver},
			{ID: "n2", Text: "中间机制", Role: roleTransmission},
			{ID: "n3", Text: "最终结果", Role: roleTransmission, IsTarget: true},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "branch",
				Priority: 1,
				Policy:   "causal_mechanism",
				Thesis:   "错误回指先出现",
				NodeIDs:  []string{"n3", "n1"},
				Edges:    []PreviewEdge{{From: "n3", To: "n1", Kind: "causal"}},
				Scope:    "section",
			},
			{
				ID:       "s2",
				Level:    "primary",
				Priority: 2,
				Policy:   "causal_mechanism",
				Thesis:   "初始驱动经由中间机制导致最终结果",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2", Kind: "causal"},
					{From: "n2", To: "n3", Kind: "causal"},
				},
				Scope: "article",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "最终结果", "初始驱动") {
		t.Fatalf("TransmissionPaths = %#v, want earlier endpoint path retained", out.TransmissionPaths)
	}
	if hasTransmissionPath(out.TransmissionPaths, "初始驱动", "最终结果") {
		t.Fatalf("TransmissionPaths = %#v, want later multi-step cycle-closing path dropped", out.TransmissionPaths)
	}
}

func TestStage5RenderKeepsSalientPathStepDisplayTargets(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"驱动"},{"id":"n2","text":"中间桥"},{"id":"n3","text":"最终结果"}]}`},
		{Text: `{"summary":"驱动通过中间桥导致最终结果。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:path-step-target",
		Source:  "web",
		Content: "A drives B, B drives C.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "驱动", Role: roleDriver},
			{ID: "n2", Text: "中间桥", Role: roleTransmission, IsTarget: true},
			{ID: "n3", Text: "最终结果", Role: roleTransmission, IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}, {From: "n2", To: "n3"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !containsString(out.Targets, "中间桥") {
		t.Fatalf("Targets = %#v, want salient path step target retained", out.Targets)
	}
	if !containsString(out.Targets, "最终结果") {
		t.Fatalf("Targets = %#v, want final path target retained", out.Targets)
	}
}

func TestStage5RenderUsesCritiqueMechanismForAllIllustrationSatireSpine(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"幸运游戏规则：2000人每人出资5万"},{"id":"n2","text":"村长贷回1亿本金"},{"id":"n3","text":"游戏结束后本金归管理者基金"},{"id":"n4","text":"通过叙事包装游戏为公平"},{"id":"n5","text":"村长必须控制叙事"}]}`},
		{Text: `{"summary":"通过叙事包装游戏为公平，最终让本金归管理者基金。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:all-illustration-satire",
		Source:  "bilibili",
		Content: "A village lottery is an allegory.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "幸运游戏规则：2000人每人出资5万", DiscourseRole: "mechanism"},
			{ID: "n2", Text: "村长贷回1亿本金", DiscourseRole: "mechanism"},
			{ID: "n3", Text: "游戏结束后本金归管理者基金", DiscourseRole: "mechanism"},
			{ID: "n4", Text: "通过叙事包装游戏为公平", DiscourseRole: "mechanism"},
			{ID: "n5", Text: "村长必须控制叙事", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2", Kind: "illustration"},
			{From: "n2", To: "n3", Kind: "illustration"},
			{From: "n3", To: "n4", Kind: "illustration"},
			{From: "n4", To: "n5", Kind: "illustration"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2", "n3", "n4", "n5"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "illustration"},
				{From: "n2", To: "n3", Kind: "illustration"},
				{From: "n3", To: "n4", Kind: "illustration"},
				{From: "n4", To: "n5", Kind: "illustration"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "幸运游戏规则：2000人每人出资5万") {
		t.Fatalf("Drivers = %#v, want story mechanics kept out of display driver", out.Drivers)
	}
	if !containsString(out.Drivers, "通过叙事包装游戏为公平") {
		t.Fatalf("Drivers = %#v, want critique mechanism as display driver", out.Drivers)
	}
	if !containsString(out.Targets, "游戏结束后本金归管理者基金") {
		t.Fatalf("Targets = %#v, want wealth-transfer consequence as display target", out.Targets)
	}
	if containsString(out.Targets, "村长贷回1亿本金") {
		t.Fatalf("Targets = %#v, want intermediate bridge omitted from display targets", out.Targets)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "通过叙事包装游戏为公平", "游戏结束后本金归管理者基金") {
		t.Fatalf("TransmissionPaths = %#v, want synthetic satire display path", out.TransmissionPaths)
	}
}

func TestStage5RenderUsesOffGraphConsequenceForCompressedSatireSpine(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"该幸运游戏本质上极其不公平"},{"id":"n2","text":"追求高回报率容易掉入陷阱"},{"id":"o1","text":"游戏结束后一个亿本金归管理者基金"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺表面公平方案隐藏财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:compressed-satire",
		Source:  "bilibili",
		Content: "A compressed satire spine.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n2", Text: "追求高回报率容易掉入陷阱", DiscourseRole: "thesis"},
		},
		OffGraph: []offGraphItem{{
			ID:         "o1",
			Text:       "游戏结束后一个亿本金归管理者基金",
			Role:       "explanation",
			AttachesTo: "n1",
		}, {
			ID:         "o2",
			Text:       "无关零售客户承担高额手续费",
			Role:       "explanation",
			AttachesTo: "other_branch",
		}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "illustration"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !containsString(out.Targets, "游戏结束后一个亿本金归管理者基金") {
		t.Fatalf("Targets = %#v, want off-graph wealth-transfer consequence", out.Targets)
	}
	if containsString(out.Targets, "追求高回报率容易掉入陷阱") {
		t.Fatalf("Targets = %#v, want generic trap target replaced by concrete consequence", out.Targets)
	}
}

func TestStage5RenderDoesNotStealUnattachedOffGraphSatireTarget(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"该幸运游戏本质上极其不公平"},{"id":"n2","text":"追求高回报率容易掉入陷阱"},{"id":"o1","text":"无关零售客户承担高额手续费"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺表面公平方案隐藏财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:unattached-offgraph",
		Source:  "bilibili",
		Content: "A compressed satire spine.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n2", Text: "追求高回报率容易掉入陷阱", DiscourseRole: "thesis"},
		},
		OffGraph: []offGraphItem{{ID: "o1", Text: "无关零售客户承担高额手续费", Role: "explanation", AttachesTo: "other_branch"}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "illustration"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Targets, "无关零售客户承担高额手续费") {
		t.Fatalf("Targets = %#v, want unattached off-graph item ignored", out.Targets)
	}
}

func TestStage5RenderIgnoresBlankAttachOffGraphSatireTarget(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"该幸运游戏本质上极其不公平"},{"id":"n2","text":"追求高回报率容易掉入陷阱"},{"id":"o1","text":"空挂载零售客户承担高额手续费"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺表面公平方案隐藏财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:blank-attach-offgraph",
		Source:  "bilibili",
		Content: "A compressed satire spine.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n2", Text: "追求高回报率容易掉入陷阱", DiscourseRole: "thesis"},
		},
		OffGraph: []offGraphItem{{ID: "o1", Text: "空挂载零售客户承担高额手续费", Role: "explanation"}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "illustration"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Targets, "空挂载零售客户承担高额手续费") {
		t.Fatalf("Targets = %#v, want blank-attached off-graph item ignored", out.Targets)
	}
}

func TestStage5RenderProjectsSatiricalBranchAlongsideCausalPrimary(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"利率下行"},{"id":"n2","text":"投资被压制"},{"id":"n3","text":"该幸运游戏本质上极其不公平"},{"id":"n4","text":"游戏结束后本金归管理者基金"}]}`},
		{Text: `{"summary":"利率下行压制投资，同时讽刺不公平游戏中的财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-branch",
		Source:  "bilibili",
		Content: "Mixed causal essay with a satire branch.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "利率下行", DiscourseRole: "mechanism"},
			{ID: "n2", Text: "投资被压制", DiscourseRole: "implication"},
			{ID: "n3", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n4", Text: "游戏结束后本金归管理者基金", DiscourseRole: "implication"},
		},
		Spines: []PreviewSpine{
			{ID: "s1", Level: "primary", Priority: 1, Policy: "causal_mechanism", Thesis: "利率压制投资", NodeIDs: []string{"n1", "n2"}, Edges: []PreviewEdge{{From: "n1", To: "n2"}}, Scope: "article"},
			{ID: "s2", Level: "branch", Priority: 2, Policy: "satirical_analogy", Thesis: "幸运游戏说明财富转移", NodeIDs: []string{"n3", "n4"}, Edges: []PreviewEdge{{From: "n3", To: "n4", Kind: "illustration"}}, Scope: "section"},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "利率下行", "投资被压制") {
		t.Fatalf("TransmissionPaths = %#v, want primary causal path retained", out.TransmissionPaths)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "该幸运游戏本质上极其不公平", "游戏结束后本金归管理者基金") {
		t.Fatalf("TransmissionPaths = %#v, want branch satire path projected", out.TransmissionPaths)
	}
}

func TestStage5RenderOmitsBridgeDriverTargetsFromDisplayTargets(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"沃什上任概率上升"},{"id":"n2","text":"大幅降息"},{"id":"n3","text":"金融抑制启动"},{"id":"n4","text":"现金购买力贬值"},{"id":"n5","text":"债券收益承压"}]}`},
		{Text: `{"summary":"沃什上任概率上升通过降息触发金融抑制，并导致现金购买力贬值。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:bridge-target",
		Source:     "youtube",
		ExternalID: "bridge-target",
		Content:    "Warsh appointment triggers financial repression, then cash purchasing power erodes.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "沃什上任概率上升"},
			{ID: "n2", Text: "大幅降息"},
			{ID: "n3", Text: "金融抑制启动"},
			{ID: "n4", Text: "现金购买力贬值"},
			{ID: "n5", Text: "债券收益承压"},
			{ID: "n6", Text: "金融抑制正式开启"},
			{ID: "n7", Text: "金融抑制的核心机制是维持负实际利率"},
			{ID: "n8", Text: "金融抑制通过存款利率上限与资本管制锁定资金购买国债"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n3", To: "n4"},
			{From: "n2", To: "n5"},
			{From: "n1", To: "n6"},
			{From: "n2", To: "n6"},
			{From: "n1", To: "n7"},
			{From: "n7", To: "n8"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Thesis:   "沃什触发金融抑制",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n3"},
				},
				Scope: "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Thesis:   "金融抑制侵蚀现金购买力",
				NodeIDs:  []string{"n3", "n4"},
				Edges: []PreviewEdge{
					{From: "n3", To: "n4"},
				},
				Scope: "section",
			},
			{
				ID:       "s3",
				Level:    "branch",
				Priority: 3,
				Thesis:   "降息冲击债券收益",
				NodeIDs:  []string{"n2", "n5"},
				Edges: []PreviewEdge{
					{From: "n2", To: "n5"},
				},
				Scope: "section",
			},
			{
				ID:       "s4",
				Level:    "branch",
				Priority: 4,
				Thesis:   "政策切换开启金融抑制",
				NodeIDs:  []string{"n1", "n2", "n6"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n6"},
				},
				Scope: "section",
			},
			{
				ID:       "s5",
				Level:    "branch",
				Priority: 5,
				Thesis:   "金融抑制机制说明",
				NodeIDs:  []string{"n1", "n7", "n8"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n7"},
					{From: "n7", To: "n8"},
				},
				Scope: "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "金融抑制启动") {
		t.Fatalf("Drivers = %#v, want bridge target-driver omitted from display drivers", out.Drivers)
	}
	if containsString(out.Drivers, "大幅降息") {
		t.Fatalf("Drivers = %#v, want transmission-step driver omitted from display drivers", out.Drivers)
	}
	if !containsString(out.Drivers, "沃什上任概率上升") {
		t.Fatalf("Drivers = %#v, want upstream source driver retained", out.Drivers)
	}
	if containsString(out.Targets, "金融抑制的核心机制是维持负实际利率") || containsString(out.Targets, "金融抑制通过存款利率上限与资本管制锁定资金购买国债") {
		t.Fatalf("Targets = %#v, want mechanism-definition targets omitted from display targets", out.Targets)
	}
	if !containsString(out.Targets, "现金购买力贬值") {
		t.Fatalf("Targets = %#v, want downstream user-facing target retained", out.Targets)
	}
	if !containsString(out.Targets, "债券收益承压") {
		t.Fatalf("Targets = %#v, want branch terminal target retained", out.Targets)
	}
	if containsString(out.Targets, "金融抑制启动") {
		t.Fatalf("Targets = %#v, want bridge financial repression conclusion omitted from display targets", out.Targets)
	}
	if containsString(out.Targets, "金融抑制正式开启") {
		t.Fatalf("Targets = %#v, want process-state financial repression conclusion omitted from display targets", out.Targets)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "沃什上任概率上升", "金融抑制启动") {
		t.Fatalf("TransmissionPaths = %#v, want evidence path into financial repression bridge conclusion", out.TransmissionPaths)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "沃什上任概率上升", "金融抑制正式开启") {
		t.Fatalf("TransmissionPaths = %#v, want evidence path into financial repression launch conclusion", out.TransmissionPaths)
	}
	if len(out.TransmissionPaths) != 5 {
		t.Fatalf("TransmissionPaths = %#v, want both spine paths retained", out.TransmissionPaths)
	}
}

func TestStage5RenderOmitsLightForecastCaveatTargets(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"沃什主张大幅降低利率"},{"id":"n2","text":"降息政策旨在制造负实际利率"},{"id":"n3","text":"实际利率转负是金融抑制启动的关键监测指标"},{"id":"n4","text":"沃什认为AI将成为反通胀力量"},{"id":"n5","text":"AI技术发展预期将压制通胀"},{"id":"n6","text":"投资者需进行多元资产配置"}]}`},
		{Text: `{"summary":"沃什主张大幅降息使实际利率转负成为金融抑制的关键监测指标，并推动多元资产配置需求。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "youtube:forecast-ai-caveat",
		Source:  "youtube",
		Content: "Warsh rate cuts matter; AI disinflation is a side tension.",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "沃什主张大幅降低利率"},
			{ID: "n2", Text: "降息政策旨在制造负实际利率"},
			{ID: "n3", Text: "实际利率转负是金融抑制启动的关键监测指标", DiscourseRole: "implication"},
			{ID: "n4", Text: "沃什认为AI将成为反通胀力量"},
			{ID: "n5", Text: "AI技术发展预期将压制通胀", DiscourseRole: "mechanism"},
			{ID: "n6", Text: "投资者需进行多元资产配置", DiscourseRole: "implication"},
			{ID: "n7", Text: "跨境税务结构可能大幅侵蚀资产实际收益", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n4", To: "n5"},
			{From: "n3", To: "n6"},
			{From: "n3", To: "n7"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Thesis:   "沃什降息推动金融抑制判断",
				NodeIDs:  []string{"n1", "n2", "n3", "n6"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n3"},
					{From: "n3", To: "n6"},
				},
				Scope: "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Thesis:   "AI反通胀是沃什叙事旁支",
				NodeIDs:  []string{"n4", "n5"},
				Edges: []PreviewEdge{
					{From: "n4", To: "n5"},
				},
				Scope: "section",
			},
			{
				ID:       "s3",
				Level:    "branch",
				Priority: 3,
				Thesis:   "跨境税务是配置执行旁支",
				NodeIDs:  []string{"n3", "n7"},
				Edges: []PreviewEdge{
					{From: "n3", To: "n7"},
				},
				Scope: "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Targets, "AI技术发展预期将压制通胀") {
		t.Fatalf("Targets = %#v, want light AI disinflation caveat omitted", out.Targets)
	}
	if containsString(out.Targets, "跨境税务结构可能大幅侵蚀资产实际收益") {
		t.Fatalf("Targets = %#v, want cross-border tax side note omitted", out.Targets)
	}
	if !containsString(out.Targets, "投资者需进行多元资产配置") {
		t.Fatalf("Targets = %#v, want downstream policy implication retained", out.Targets)
	}
}

func TestStage5RenderRecoversTargetFromOffGraphWhenMainlineHasOnlyDriver(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"石油美元闭环形成"},{"id":"off1","text":"私募信贷赎回门和流动性挤兑风险上升"},{"id":"off2","text":"中东资金减少购买美债美股"}]}`},
		{Text: `{"summary":"石油美元闭环受冲击，私募信贷赎回门和流动性挤兑风险上升。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "weibo:QAJ0n0YGU",
		Source:     "weibo",
		ExternalID: "QAJ0n0YGU",
		Content:    "石油美元闭环变化可能导致私募信贷赎回门和流动性挤兑。",
	}, graphState{
		Nodes: []graphNode{{
			ID:   "n1",
			Text: "石油美元闭环形成",
			Role: roleDriver,
		}},
		OffGraph: []offGraphItem{
			{
				ID:         "off1",
				Text:       "私募信贷赎回门和流动性挤兑风险上升",
				Role:       "evidence",
				AttachesTo: "n1",
			},
			{
				ID:         "off2",
				Text:       "中东资金减少购买美债美股",
				Role:       "supplementary",
				AttachesTo: "n1",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Targets) == 0 {
		t.Fatalf("Targets = %#v, want fallback target recovered from off_graph", out.Targets)
	}
	if out.Targets[0] != "私募信贷赎回门和流动性挤兑风险上升" {
		t.Fatalf("Targets[0] = %q, want private-credit liquidity target", out.Targets[0])
	}
}

func TestStage5RenderDirectPathOmitsSyntheticDriverStep(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"驱动A"},{"id":"n2","text":"目标B"}]}`},
		{Text: `{"summary":"驱动A推动目标B。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "web:direct-path",
		Source:     "web",
		ExternalID: "direct-path",
		Content:    "Driver A drives target B.",
		PostedAt:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Driver A", Role: roleDriver},
			{ID: "n2", Text: "Target B", Role: roleTransmission, IsTarget: true, Ontology: "flow"},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want one direct path", out.TransmissionPaths)
	}
	if len(out.TransmissionPaths[0].Steps) != 0 {
		t.Fatalf("TransmissionPaths[0].Steps = %#v, want direct path to omit synthetic driver step", out.TransmissionPaths[0].Steps)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Output.Validate() error = %v", err)
	}
}
