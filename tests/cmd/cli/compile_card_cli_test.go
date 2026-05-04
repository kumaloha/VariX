package main

import (
	"bytes"
	"context"
	"database/sql"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest"
	"github.com/kumaloha/VariX/varix/ingest/dispatcher"
	"github.com/kumaloha/VariX/varix/ingest/types"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCompileCardPrintsHumanReadableCard(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话总结",
				Drivers: []string{"驱动A"},
				Targets: []string{"目标B"},
				Declarations: []c.Declaration{{
					ID:          "decl-1",
					Speaker:     "Greg Abel",
					Kind:        "capital_allocation_rule",
					Topic:       "capital_allocation",
					Statement:   "伯克希尔会等待市场错配",
					Conditions:  []string{"市场出现错配"},
					Actions:     []string{"快速且果断行动"},
					Scale:       "投入大量资本",
					Constraints: []string{"不会仅因现金规模大而被迫投资"},
					Evidence:    []string{"现金和短债约3800亿美元"},
					SourceQuote: "there will be dislocations in markets ... act decisively both quickly and with significant capital",
					Confidence:  "high",
				}},
				SemanticUnits: []c.SemanticUnit{{
					ID:               "u-portfolio",
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
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				Branches: []c.Branch{{
					ID:            "s1",
					Level:         "primary",
					Policy:        "capital_allocation_rule",
					Thesis:        "分支论点",
					Anchors:       []string{"总前提"},
					BranchDrivers: []string{"分支机制"},
					Drivers:       []string{"驱动A"},
					Targets:       []string{"目标B"},
					Declarations: []c.Declaration{{
						ID:         "decl-1",
						Speaker:    "Greg Abel",
						Kind:       "capital_allocation_rule",
						Topic:      "capital_allocation",
						Statement:  "伯克希尔会等待市场错配",
						Conditions: []string{"市场出现错配"},
						Actions:    []string{"快速且果断行动"},
						Scale:      "投入大量资本",
					}},
					TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				}},
				EvidenceNodes:    []string{"证据A"},
				ExplanationNodes: []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Topics:     []string{"topic-a", "topic-b"},
				Confidence: "high",
				AuthorValidation: c.AuthorValidation{
					Version: "author_claim_validation",
					Summary: c.AuthorValidationSummary{
						Verdict:               "mixed",
						SupportedClaims:       1,
						UnverifiedClaims:      1,
						SoundInferences:       1,
						UnsupportedInferences: 1,
					},
					ClaimChecks: []c.AuthorClaimCheck{{
						ClaimID: "claim-001",
						Text:    "目标B",
						Status:  c.AuthorClaimUnverified,
						Reason:  "缺少外部证据",
					}},
					InferenceChecks: []c.AuthorInferenceCheck{{
						InferenceID: "inference-001",
						From:        "驱动A",
						To:          "目标B",
						Status:      c.AuthorInferenceUnsupportedJump,
						Reason:      "中间条件不成立",
					}},
				},
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Topics", "topic-a", "Management declarations", "Greg Abel", "capital_allocation", "伯克希尔会等待市场错配", "Read: 这回答的是“Greg Abel 会怎么用手里的钱”", "平时以不会仅因现金规模大而被迫投资为边界", "触发条件是市场出现错配", "条件满足后快速且果断行动，规模是投入大量资本", "Condition: 市场出现错配", "Action: 快速且果断行动", "Scale: 投入大量资本", "Boundary: 不会仅因现金规模大而被迫投资", "Evidence: 现金和短债约3800亿美元", "Speaker claims", "existing portfolio / circle of competence", "Question: 股东询问 Greg Abel 如何管理 Warren Buffett 建立的组合。", "Answer: 现有组合由 Warren Buffett 建立", "Apple 说明能力圈不是行业标签", "Branches", "Anchor: 总前提", "Branch driver: 分支机制", "Declaration: 伯克希尔会等待市场错配", "驱动A -> 中间步骤 -> 目标B", "Logic chain", "Author validation", "Verdict: mixed", "Claims: supported 1, contradicted 0, unverified 1, interpretive 0", "Claim 目标B: unverified — 说明: 缺少外部证据", "Path 驱动A -> 目标B: unsupported_jump — 说明: 中间条件不成立", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardShowsGraphFirstExpandedViewAndVerificationSummary(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:expanded-view",
			Source:         "twitter",
			ExternalID:     "expanded-view",
			RootExternalID: "expanded-view",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据"},
				ExplanationNodes:  []string{"旧解释"},
				Topics:            []string{"topic-a", "topic-b"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := model.ContentSubgraph{
			ID:               "twitter:expanded-view",
			ArticleID:        "twitter:expanded-view",
			SourcePlatform:   "twitter",
			SourceExternalID: "expanded-view",
			RootExternalID:   "expanded-view",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "n1", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationProved},
				{ID: "n2", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "CPI回落", SubjectText: "CPI", ChangeText: "回落", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleEvidence, IsPrimary: false, VerificationStatus: model.VerificationProved},
				{ID: "n5", SourceArticleID: "twitter:expanded-view", SourcePlatform: "twitter", SourceExternalID: "expanded-view", RawText: "估值承压先传导到科技股", SubjectText: "科技股估值", ChangeText: "承压", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleContext, IsPrimary: false, VerificationStatus: model.VerificationUnverifiable},
			},
			Edges: []model.ContentEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationProved},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n4->n1:supports", From: "n4", To: "n1", Type: model.EdgeTypeSupports, IsPrimary: false, VerificationStatus: model.VerificationProved},
				{ID: "n5->n3:explains", From: "n5", To: "n3", Type: model.EdgeTypeExplains, IsPrimary: false, VerificationStatus: model.VerificationUnverifiable},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "expanded-view"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Summary", "一句话总结",
		"Topics", "topic-a",
		"Drivers", "- 美联储加息0.25%",
		"Targets", "- 未来一周美股承压",
		"Evidence", "- CPI回落",
		"Explanations", "- 估值承压先传导到科技股",
		"Logic chain", "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压",
		"Verification",
		"Nodes: pending=2, proved=2, unverifiable=1",
		"Edges: pending=1, proved=2, unverifiable=1",
		"Confidence", "high",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardFallsBackToLegacyWhenGraphFirstSubgraphMissing(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:legacy-fallback-standard",
			Source:         "twitter",
			ExternalID:     "legacy-fallback-standard",
			RootExternalID: "legacy-fallback-standard",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Topics:            []string{"主题A", "主题B"},
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DELETE FROM content_subgraphs WHERE platform = ? AND external_id = ?`, "twitter", "legacy-fallback-standard"); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "legacy-fallback-standard"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Logic chain", "驱动A -> 中间步骤 -> 目标B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "Verification") {
		t.Fatalf("stdout = %q, did not want verification summary without subgraph", out)
	}
}

func TestRunCompileCardFallsBackToLegacyWhenGraphFirstProjectionIsLessInformative(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:less-informative-standard",
			Source:         "twitter",
			ExternalID:     "less-informative-standard",
			RootExternalID: "less-informative-standard",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动A", "旧驱动B"},
				Targets:           []string{"旧目标A", "旧目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动A", Target: "旧目标A", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据A", "旧证据B"},
				ExplanationNodes:  []string{"旧解释A", "旧解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标A")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := model.ContentSubgraph{
			ID:               "twitter:less-informative-standard",
			ArticleID:        "twitter:less-informative-standard",
			SourcePlatform:   "twitter",
			SourceExternalID: "less-informative-standard",
			RootExternalID:   "less-informative-standard",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "n1", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新驱动", SubjectText: "新驱动", ChangeText: "新驱动", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新目标", SubjectText: "新目标", ChangeText: "新目标", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新证据", SubjectText: "新证据", ChangeText: "新证据", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleEvidence, IsPrimary: false, VerificationStatus: model.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:less-informative-standard", SourcePlatform: "twitter", SourceExternalID: "less-informative-standard", RawText: "新解释", SubjectText: "新解释", ChangeText: "新解释", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleContext, IsPrimary: false, VerificationStatus: model.VerificationPending},
			},
			Edges: []model.ContentEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "less-informative-standard"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 旧驱动A", "- 旧驱动B", "- 旧目标A", "- 旧目标B", "- 旧证据A", "- 旧证据B", "- 旧解释A", "- 旧解释B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing legacy section item %q in %q", want, out)
		}
	}
	for _, avoid := range []string{"- 新驱动", "- 新目标", "- 新证据", "- 新解释"} {
		if strings.Contains(out, avoid) {
			t.Fatalf("stdout = %q, did not want less-informative graph-first item %q", out, avoid)
		}
	}
}

func TestRunCompileCardCompactPrintsCompactView(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Topics:            []string{"主题A", "主题B"},
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Topics", "主题A", "Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Main logic", "驱动A -> 中间步骤 -> 目标B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardCompactReadsByURL(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "QAu4U9USk", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:QAu4U9USk",
			Source:         "weibo",
			ExternalID:     "QAu4U9USk",
			RootExternalID: "QAu4U9USk",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--url", "https://weibo.com/1182426800/QAu4U9USk"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary", "一句话总结", "Drivers", "- 驱动A", "Targets", "- 目标B", "Evidence", "- 证据A", "Explanations", "- 解释B", "Main logic", "驱动A -> 中间步骤 -> 目标B", "Confidence", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardPrefersGraphFirstLogicChain(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-logic",
			Source:         "twitter",
			ExternalID:     "graph-first-logic",
			RootExternalID: "graph-first-logic",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := model.ContentSubgraph{
			ID:               "twitter:graph-first-logic",
			ArticleID:        "twitter:graph-first-logic",
			SourcePlatform:   "twitter",
			SourceExternalID: "graph-first-logic",
			RootExternalID:   "graph-first-logic",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "n1", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:graph-first-logic", SourcePlatform: "twitter", SourceExternalID: "graph-first-logic", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending},
			},
			Edges: []model.ContentEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "graph-first-logic"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压") {
		t.Fatalf("stdout = %q, want graph-first logic chain", out)
	}
	if strings.Contains(out, "旧驱动 -> 旧中间步骤 -> 旧目标") {
		t.Fatalf("stdout = %q, did not want stale legacy logic chain", out)
	}
}

func TestRunCompileCardCompactPrefersGraphFirstProjection(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-compact",
			Source:         "twitter",
			ExternalID:     "graph-first-compact",
			RootExternalID: "graph-first-compact",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动"},
				Targets:           []string{"旧目标"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动", Target: "旧目标", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据"},
				ExplanationNodes:  []string{"旧解释"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := model.ContentSubgraph{
			ID:               "twitter:graph-first-compact",
			ArticleID:        "twitter:graph-first-compact",
			SourcePlatform:   "twitter",
			SourceExternalID: "graph-first-compact",
			RootExternalID:   "graph-first-compact",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "n1", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "流动性收紧", SubjectText: "流动性", ChangeText: "收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleIntermediate, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "CPI回落", SubjectText: "CPI", ChangeText: "回落", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleEvidence, IsPrimary: false, VerificationStatus: model.VerificationPending},
				{ID: "n5", SourceArticleID: "twitter:graph-first-compact", SourcePlatform: "twitter", SourceExternalID: "graph-first-compact", RawText: "估值承压先传导到科技股", SubjectText: "科技股估值", ChangeText: "承压", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleContext, IsPrimary: false, VerificationStatus: model.VerificationPending},
			},
			Edges: []model.ContentEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n2->n3:drives", From: "n2", To: "n3", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n4->n1:supports", From: "n4", To: "n1", Type: model.EdgeTypeSupports, IsPrimary: false, VerificationStatus: model.VerificationPending},
				{ID: "n5->n3:explains", From: "n5", To: "n3", Type: model.EdgeTypeExplains, IsPrimary: false, VerificationStatus: model.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "graph-first-compact"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 美联储加息0.25%", "- 未来一周美股承压", "- CPI回落", "- 估值承压先传导到科技股", "美联储加息0.25% -> 流动性收紧 -> 未来一周美股承压"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
	for _, stale := range []string{"- 旧驱动", "- 旧目标", "- 旧证据", "- 旧解释", "旧驱动 -> 旧中间步骤 -> 旧目标"} {
		if strings.Contains(out, stale) {
			t.Fatalf("stdout = %q, did not want stale legacy projection %q", out, stale)
		}
	}
}

func TestRunCompileCardCompactFallsBackToLegacyWhenGraphFirstSubgraphMissing(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:legacy-fallback-card",
			Source:         "twitter",
			ExternalID:     "legacy-fallback-card",
			RootExternalID: "legacy-fallback-card",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DELETE FROM content_subgraphs WHERE platform = ? AND external_id = ?`, "twitter", "legacy-fallback-card"); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "legacy-fallback-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 驱动A", "- 目标B", "- 证据A", "- 解释B", "驱动A -> 中间步骤 -> 目标B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q in %q", want, out)
		}
	}
}

func TestRunCompileCardFailsWhenGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-store-error",
			Source:         "twitter",
			ExternalID:     "graph-first-store-error",
			RootExternalID: "graph-first-store-error",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "graph-first-store-error"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardFailsWhenExpandedGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:graph-first-store-error-expanded",
			Source:         "twitter",
			ExternalID:     "graph-first-store-error-expanded",
			RootExternalID: "graph-first-store-error-expanded",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "graph-first-store-error-expanded"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardFailsWhenURLGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "Q-store-url", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-store-url",
			Source:         "weibo",
			ExternalID:     "Q-store-url",
			RootExternalID: "Q-store-url",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--url", "https://weibo.com/1182426800/Q-store-url"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardCompactFailsWhenURLGraphFirstLookupReturnsUnexpectedStoreError(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "content.db")
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = dbPath
		app.Dispatcher = dispatcher.New(
			func(raw string) (types.ParsedURL, error) {
				return types.ParsedURL{Platform: types.PlatformWeibo, PlatformID: "Q-store-url-compact", CanonicalURL: raw}, nil
			},
			[]dispatcher.ItemSource{fakeItemSource{}},
			nil,
			nil,
		)
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "weibo:Q-store-url-compact",
			Source:         "weibo",
			ExternalID:     "Q-store-url-compact",
			RootExternalID: "Q-store-url-compact",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"驱动A"},
				Targets:           []string{"目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "驱动A", Target: "目标B", Steps: []string{"中间步骤"}}},
				EvidenceNodes:     []string{"证据A"},
				ExplanationNodes:  []string{"解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "目标B")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		rawDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		defer rawDB.Close()
		if _, err := rawDB.Exec(`DROP TABLE content_subgraphs`); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--url", "https://weibo.com/1182426800/Q-store-url-compact"}, "/tmp/project", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1, stdout = %s, stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "content_subgraphs") {
		t.Fatalf("stderr = %q, want surfaced content_subgraphs lookup error", stderr.String())
	}
}

func TestRunCompileCardCompactFallsBackToLegacyWhenGraphFirstProjectionIsLessInformative(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:less-informative-card",
			Source:         "twitter",
			ExternalID:     "less-informative-card",
			RootExternalID: "less-informative-card",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary:           "一句话总结",
				Drivers:           []string{"旧驱动A", "旧驱动B"},
				Targets:           []string{"旧目标A", "旧目标B"},
				TransmissionPaths: []c.TransmissionPath{{Driver: "旧驱动A", Target: "旧目标A", Steps: []string{"旧中间步骤"}}},
				EvidenceNodes:     []string{"旧证据A", "旧证据B"},
				ExplanationNodes:  []string{"旧解释A", "旧解释B"},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("legacy-driver", c.NodeMechanism, "旧驱动A"), testGraphNode("legacy-target", c.NodeConclusion, "旧目标A")},
					Edges: []c.GraphEdge{{From: "legacy-driver", To: "legacy-target", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		subgraph := model.ContentSubgraph{
			ID:               "twitter:less-informative-card",
			ArticleID:        "twitter:less-informative-card",
			SourcePlatform:   "twitter",
			SourceExternalID: "less-informative-card",
			RootExternalID:   "less-informative-card",
			CompileVersion:   model.CompileBridgeVersion,
			CompiledAt:       time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
			Nodes: []model.ContentNode{
				{ID: "n1", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新驱动", SubjectText: "新驱动", ChangeText: "新驱动", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n2", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新目标", SubjectText: "新目标", ChangeText: "新目标", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending},
				{ID: "n3", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新证据", SubjectText: "新证据", ChangeText: "新证据", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleEvidence, IsPrimary: false, VerificationStatus: model.VerificationPending},
				{ID: "n4", SourceArticleID: "twitter:less-informative-card", SourcePlatform: "twitter", SourceExternalID: "less-informative-card", RawText: "新解释", SubjectText: "新解释", ChangeText: "新解释", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleContext, IsPrimary: false, VerificationStatus: model.VerificationPending},
			},
			Edges: []model.ContentEdge{
				{ID: "n1->n2:drives", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending},
			},
		}
		if err := store.UpsertContentSubgraph(context.Background(), subgraph); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--compact", "--platform", "twitter", "--id", "less-informative-card"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"- 旧驱动A", "- 旧驱动B", "- 旧目标A", "- 旧目标B", "- 旧证据A", "- 旧证据B", "- 旧解释A", "- 旧解释B"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing legacy section item %q in %q", want, out)
		}
	}
	for _, avoid := range []string{"- 新驱动", "- 新目标", "- 新证据", "- 新解释"} {
		if strings.Contains(out, avoid) {
			t.Fatalf("stdout = %q, did not want less-informative graph-first item %q", out, avoid)
		}
	}
}

func TestRunCompileCardCollapsesLinearChain(t *testing.T) {
	prevNewIngestRuntime := newIngestRuntime
	prevOpenSQLiteStore := openSQLiteStore
	t.Cleanup(func() {
		newIngestRuntime = prevNewIngestRuntime
		openSQLiteStore = prevOpenSQLiteStore
	})

	tmp := t.TempDir()
	newIngestRuntime = func(projectRoot string) (*ingest.Runtime, error) {
		app := &ingest.Runtime{}
		app.Settings.ContentDBPath = tmp + "/content.db"
		return app, nil
	}
	openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
		store, err := contentstore.NewSQLiteStore(path)
		if err != nil {
			return nil, err
		}
		record := c.Record{
			UnitID:         "twitter:1",
			Source:         "twitter",
			ExternalID:     "1",
			RootExternalID: "1",
			Model:          varixllm.Qwen36PlusModel,
			Output: c.Output{
				Summary: "一句话总结",
				Drivers: []string{"驱动A"},
				Targets: []string{"目标C"},
				TransmissionPaths: []c.TransmissionPath{{
					Driver: "驱动A",
					Target: "目标C",
					Steps:  []string{"步骤B"},
				}},
				Graph: c.ReasoningGraph{
					Nodes: []c.GraphNode{testGraphNode("n1", c.NodeMechanism, "驱动A"), testGraphNode("n2", c.NodeMechanism, "步骤B"), testGraphNode("n3", c.NodeConclusion, "目标C")},
					Edges: []c.GraphEdge{{From: "n1", To: "n2", Kind: c.EdgePositive}, {From: "n2", To: "n3", Kind: c.EdgePositive}},
				},
				Details:    c.HiddenDetails{Caveats: []string{"说明"}},
				Confidence: "high",
			},
			CompiledAt: time.Now().UTC(),
		}
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			return nil, err
		}
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"compile", "card", "--platform", "twitter", "--id", "1"}, "/tmp/project", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "驱动A -> 步骤B -> 目标C") {
		t.Fatalf("stdout missing collapsed chain in %q", out)
	}
}
