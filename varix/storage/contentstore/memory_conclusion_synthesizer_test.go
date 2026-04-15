package contentstore

import (
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func TestBuildCognitiveConclusion_AllowsSingleSourceCompleteChain(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CoreQuestion:      "关于「流动性收紧与风险资产」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险资产承压",
		Summary:         "流动性收紧 → 风险资产承压",
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got.Headline == "" {
		t.Fatalf("Headline = empty, want synthesized conclusion")
	}
	if len(got.BackingCardIDs) != 1 || got.BackingCardIDs[0] != "card-1" {
		t.Fatalf("BackingCardIDs = %#v, want card-1", got.BackingCardIDs)
	}
}

func TestBuildCognitiveConclusion_RejectsGenericHeadline(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CorePathNodeIDs:   []string{"n1", "n2"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险值得关注",
		Summary:         "市场可能发生变化",
		ConfidenceLabel: "strong",
	}}

	_, ok := buildCognitiveConclusion(thesis, cards)
	if ok {
		t.Fatalf("ok = true, want false for generic headline")
	}
}

func TestBuildTopMemoryItems_PrioritizesConflict(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	conflicts := []memory.ConflictSet{{
		ConflictID:     "conflict-1",
		ConflictStatus: "blocked",
		ConflictTopic:  "关于「油价方向」的矛盾",
		UpdatedAt:      now,
	}}
	conclusions := []memory.CognitiveConclusion{{
		ConclusionID: "conclusion-1",
		Headline:     "流动性收紧会压制风险资产",
	}}

	got := buildTopMemoryItems(conflicts, conclusions, now)
	if len(got) != 2 {
		t.Fatalf("len(buildTopMemoryItems) = %d, want 2", len(got))
	}
	if got[0].ItemType != "conflict" {
		t.Fatalf("first ItemType = %q, want conflict", got[0].ItemType)
	}
}

func TestBuildTopMemoryItems_SetsSignalStrength(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	conclusions := []memory.CognitiveConclusion{{
		ConclusionID: "conclusion-1",
		Headline:     "流动性收紧正在把风险资产推向承压与更高波动",
		Subheadline:  "流动性收紧 → 风险资产承压 → 未来数月波动加大",
	}}

	got := buildTopMemoryItems(nil, conclusions, now)
	if len(got) != 1 {
		t.Fatalf("len(buildTopMemoryItems) = %d, want 1", len(got))
	}
	if got[0].SignalStrength != "high" {
		t.Fatalf("SignalStrength = %q, want high for strong abstract conclusion", got[0].SignalStrength)
	}
}

func TestBuildTopMemoryItems_HumanizesConflictReason(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	conflicts := []memory.ConflictSet{{
		ConflictID:     "conflict-1",
		ConflictStatus: "blocked",
		ConflictTopic:  "关于「油价」的判断",
		ConflictReason: "antonym contradiction",
		UpdatedAt:      now,
	}}

	got := buildTopMemoryItems(conflicts, nil, now)
	if len(got) != 1 {
		t.Fatalf("len(buildTopMemoryItems) = %d, want 1", len(got))
	}
	if got[0].Subheadline == "antonym contradiction" {
		t.Fatalf("Subheadline = %q, want human-readable conflict wording", got[0].Subheadline)
	}
}

func TestBuildCognitiveConclusion_UsesPredictionToLiftHeadline(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CoreQuestion:      "关于「流动性收紧与风险资产」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险资产承压",
		Summary:         "流动性收紧 → 风险资产承压 → 未来数月波动加大",
		KeyEvidence:     []string{"流动性收紧"},
		Predictions:     []string{"未来数月波动加大"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got.Headline == "风险资产承压" {
		t.Fatalf("Headline = %q, want lifted conclusion rather than raw card title", got.Headline)
	}
	if !strings.Contains(got.Headline, "流动性收紧") {
		t.Fatalf("Headline = %q, want key driver included in headline", got.Headline)
	}
	if got.Headline == "" {
		t.Fatalf("Headline = empty, want synthesized lifted headline")
	}
}

func TestBuildCognitiveConclusion_ProducesMoreAbstractHeadlineForPressureAndVolatility(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CoreQuestion:      "关于「流动性收紧与风险资产承压」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险资产承压",
		Summary:         "流动性收紧 → 风险资产承压 → 未来数月波动加大",
		KeyEvidence:     []string{"流动性收紧"},
		Predictions:     []string{"未来数月波动加大"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := "流动性收紧正在把风险资产推向承压与更高波动"
	if got.Headline != want {
		t.Fatalf("Headline = %q, want %q", got.Headline, want)
	}
}
