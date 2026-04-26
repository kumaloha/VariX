package compile

import "testing"

func TestScoreGoldDatasetReportsMissingAndExtraItemsForReview(t *testing.T) {
	dataset := GoldDataset{
		Version: "test-v1",
		Samples: []GoldSample{{
			ID:      "G04",
			Title:   "flows",
			Summary: "海外资金继续流入美国资产，说明美国增长叙事仍然吸引全球资金",
			Drivers: []string{
				"美国增长叙事仍然吸引全球资金",
				"政治风险没有压倒市场对美国资产的增长偏好",
			},
			Targets: []string{
				"海外资金继续流入美国资产",
				"没有形成 sell America 交易",
			},
		}},
	}
	candidate := Output{
		Summary: "美国政治风险导致美元走弱，美联储被迫降息",
		Drivers: []string{
			"美联储政治化压低收益率",
			"美国增长叙事吸引全球资金",
		},
		Targets: []string{
			"海外资金继续流入美国资产",
			"美元下跌",
		},
	}

	card := ScoreGoldDataset(dataset, []GoldCandidate{{SampleID: "G04", Output: candidate}})

	if card.SampleCount != 1 {
		t.Fatalf("SampleCount = %d, want 1", card.SampleCount)
	}
	if card.OverallScore <= 0 || card.OverallScore >= 100 {
		t.Fatalf("OverallScore = %.2f, want partial score", card.OverallScore)
	}
	got := card.Samples[0]
	if got.Drivers.MatchedCount != 1 || got.Drivers.MissingCount != 1 || got.Drivers.ExtraCount != 1 {
		t.Fatalf("driver score = %#v, want one match, one missing, one extra", got.Drivers)
	}
	if got.Targets.MatchedCount != 1 || got.Targets.MissingCount != 1 || got.Targets.ExtraCount != 1 {
		t.Fatalf("target score = %#v, want one match, one missing, one extra", got.Targets)
	}
	if !hasGoldReviewItem(got.ReviewItems, "summary", "low_alignment") {
		t.Fatalf("review items missing low summary alignment: %#v", got.ReviewItems)
	}
	if !hasGoldReviewItem(got.ReviewItems, "drivers", "missing_gold") {
		t.Fatalf("review items missing driver missing_gold: %#v", got.ReviewItems)
	}
	if !hasGoldReviewItem(got.ReviewItems, "targets", "extra_candidate") {
		t.Fatalf("review items missing target extra_candidate: %#v", got.ReviewItems)
	}
}

func TestScoreGoldDatasetFlagsMissingCandidateAsReviewCase(t *testing.T) {
	dataset := GoldDataset{
		Version: "test-v1",
		Samples: []GoldSample{{
			ID:      "G01",
			Summary: "summary",
			Drivers: []string{"driver"},
			Targets: []string{"target"},
		}},
	}

	card := ScoreGoldDataset(dataset, nil)

	if got := card.Samples[0].OverallScore; got != 0 {
		t.Fatalf("sample score = %.2f, want 0 for missing candidate", got)
	}
	if !hasGoldReviewItem(card.Samples[0].ReviewItems, "sample", "missing_candidate") {
		t.Fatalf("review items missing missing_candidate: %#v", card.Samples[0].ReviewItems)
	}
}

func TestScoreGoldDatasetMatchesBilingualFinanceConcepts(t *testing.T) {
	dataset := GoldDataset{
		Version: "test-v1",
		Samples: []GoldSample{{
			ID:      "G04",
			Summary: "海外资金继续流入美国资产，说明美国增长叙事仍然吸引全球资金",
			Drivers: []string{
				"美国增长叙事仍然吸引全球资金",
			},
			Targets: []string{
				"海外资金继续流入美国资产",
			},
		}},
	}
	candidate := Output{
		Summary: "Foreign portfolio inflows into US assets remain huge because US exceptionalism and the growth narrative still attract global capital.",
		Drivers: []string{
			"US exceptionalism and strong growth narrative",
		},
		Targets: []string{
			"Foreign portfolio inflows into US assets remain at multi-decade highs",
		},
		TransmissionPaths: []TransmissionPath{{Driver: "driver", Target: "target", Steps: []string{"step"}}},
	}

	card := ScoreGoldDataset(dataset, []GoldCandidate{{SampleID: "G04", Output: candidate}})

	got := card.Samples[0]
	if got.Drivers.MatchedCount != 1 {
		t.Fatalf("drivers matched_count = %d, want 1; score=%#v", got.Drivers.MatchedCount, got.Drivers)
	}
	if got.Targets.MatchedCount != 1 {
		t.Fatalf("targets matched_count = %d, want 1; score=%#v", got.Targets.MatchedCount, got.Targets)
	}
}

func hasGoldReviewItem(items []GoldReviewItem, field, kind string) bool {
	for _, item := range items {
		if item.Field == field && item.Kind == kind {
			return true
		}
	}
	return false
}
