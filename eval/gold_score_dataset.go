package eval

import (
	"sort"
	"strings"
)

func ScoreGoldDataset(dataset GoldDataset, candidates []GoldCandidate) GoldScorecard {
	byID := map[string]Output{}
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.SampleID)
		if id == "" {
			continue
		}
		byID[id] = candidate.Output
	}

	card := GoldScorecard{
		DatasetVersion: dataset.Version,
		SampleCount:    len(dataset.Samples),
		Samples:        make([]GoldSampleScore, 0, len(dataset.Samples)),
	}
	for _, sample := range dataset.Samples {
		candidate, ok := byID[sample.ID]
		if !ok {
			score := missingCandidateScore(sample)
			card.Samples = append(card.Samples, score)
			continue
		}
		card.Samples = append(card.Samples, scoreGoldSample(sample, candidate))
	}
	card.Rollup = buildGoldScoreRollup(card.Samples)
	card.OverallScore = roundScore(weightedOverall(card.Rollup.SummaryScore, card.Rollup.DriversScore, card.Rollup.TargetsScore, card.Rollup.StructureScore))
	return card
}

func missingCandidateScore(sample GoldSample) GoldSampleScore {
	return GoldSampleScore{
		ID:    sample.ID,
		Title: sample.Title,
		Drivers: GoldListScore{
			GoldCount:    len(sample.Drivers),
			MissingCount: len(sample.Drivers),
			MissingGold:  append([]string(nil), sample.Drivers...),
		},
		Targets: GoldListScore{
			GoldCount:    len(sample.Targets),
			MissingCount: len(sample.Targets),
			MissingGold:  append([]string(nil), sample.Targets...),
		},
		ReviewItems: []GoldReviewItem{{
			Severity: "high",
			Field:    "sample",
			Kind:     "missing_candidate",
			Message:  "No compile candidate was provided for this gold sample.",
		}},
	}
}

func scoreGoldSample(sample GoldSample, candidate Output) GoldSampleScore {
	summaryScore := roundScore(100 * textSimilarity(sample.Summary, candidate.Summary))
	drivers := scoreGoldList(sample.Drivers, candidate.Drivers)
	targets := scoreGoldList(sample.Targets, candidate.Targets)
	structureScore := scoreCandidateStructure(candidate)
	reviewItems := make([]GoldReviewItem, 0)
	if summaryScore < 70 {
		reviewItems = append(reviewItems, GoldReviewItem{
			Severity:  severityForScore(summaryScore),
			Field:     "summary",
			Kind:      "low_alignment",
			Score:     summaryScore,
			Gold:      sample.Summary,
			Candidate: candidate.Summary,
			Message:   "Candidate summary has low fuzzy alignment with the current gold summary.",
		})
	}
	reviewItems = append(reviewItems, reviewItemsForList("drivers", drivers)...)
	reviewItems = append(reviewItems, reviewItemsForList("targets", targets)...)
	if structureScore < 75 {
		reviewItems = append(reviewItems, GoldReviewItem{
			Severity: "medium",
			Field:    "structure",
			Kind:     "weak_output_shape",
			Score:    structureScore,
			Message:  "Compile output is structurally thin: summary, drivers, targets, or paths are missing.",
		})
	}
	return GoldSampleScore{
		ID:             sample.ID,
		Title:          sample.Title,
		OverallScore:   roundScore(weightedOverall(summaryScore, drivers.Score, targets.Score, structureScore)),
		SummaryScore:   summaryScore,
		Drivers:        drivers,
		Targets:        targets,
		StructureScore: structureScore,
		ReviewItems:    reviewItems,
	}
}

func scoreGoldList(gold, candidate []string) GoldListScore {
	gold = cleanStringList(gold)
	candidate = cleanStringList(candidate)
	matches, missingGold, extraCandidate := matchGoldList(gold, candidate)
	precision := ratio(len(matches), len(candidate))
	recall := ratio(len(matches), len(gold))
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return GoldListScore{
		Score:          roundScore(100 * f1),
		Precision:      roundRatio(precision),
		Recall:         roundRatio(recall),
		F1:             roundRatio(f1),
		GoldCount:      len(gold),
		CandidateCount: len(candidate),
		MatchedCount:   len(matches),
		MissingCount:   len(missingGold),
		ExtraCount:     len(extraCandidate),
		Matches:        matches,
		MissingGold:    missingGold,
		ExtraCandidate: extraCandidate,
	}
}

func matchGoldList(gold, candidate []string) ([]GoldMatch, []string, []string) {
	type pair struct {
		goldIdx      int
		candidateIdx int
		similarity   float64
	}
	pairs := make([]pair, 0, len(gold)*len(candidate))
	for i, goldItem := range gold {
		for j, candidateItem := range candidate {
			sim := textSimilarity(goldItem, candidateItem)
			if sim >= goldMatchThreshold {
				pairs = append(pairs, pair{goldIdx: i, candidateIdx: j, similarity: sim})
			}
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].similarity > pairs[j].similarity
	})
	usedGold := map[int]struct{}{}
	usedCandidate := map[int]struct{}{}
	matches := make([]GoldMatch, 0)
	for _, p := range pairs {
		if _, ok := usedGold[p.goldIdx]; ok {
			continue
		}
		if _, ok := usedCandidate[p.candidateIdx]; ok {
			continue
		}
		usedGold[p.goldIdx] = struct{}{}
		usedCandidate[p.candidateIdx] = struct{}{}
		matches = append(matches, GoldMatch{
			Gold:       gold[p.goldIdx],
			Candidate:  candidate[p.candidateIdx],
			Similarity: roundRatio(p.similarity),
		})
	}
	missing := make([]string, 0)
	for i, item := range gold {
		if _, ok := usedGold[i]; !ok {
			missing = append(missing, item)
		}
	}
	extra := make([]string, 0)
	for i, item := range candidate {
		if _, ok := usedCandidate[i]; !ok {
			extra = append(extra, item)
		}
	}
	return matches, missing, extra
}

func reviewItemsForList(field string, score GoldListScore) []GoldReviewItem {
	items := make([]GoldReviewItem, 0, score.MissingCount+score.ExtraCount)
	for _, gold := range score.MissingGold {
		items = append(items, GoldReviewItem{
			Severity: "high",
			Field:    field,
			Kind:     "missing_gold",
			Gold:     gold,
			Message:  "Gold item was not matched by the compile output; review whether compile missed it or gold is too strict.",
		})
	}
	for _, extra := range score.ExtraCandidate {
		items = append(items, GoldReviewItem{
			Severity:  "medium",
			Field:     field,
			Kind:      "extra_candidate",
			Candidate: extra,
			Message:   "Compile output produced an unmatched item; review whether it is valid or drift.",
		})
	}
	return items
}

func scoreCandidateStructure(candidate Output) float64 {
	score := 0.0
	if strings.TrimSpace(candidate.Summary) != "" {
		score += 25
	}
	if len(cleanStringList(candidate.Drivers)) > 0 {
		score += 25
	}
	if len(cleanStringList(candidate.Targets)) > 0 {
		score += 25
	}
	if len(candidate.TransmissionPaths) > 0 {
		score += 25
	}
	return score
}

func buildGoldScoreRollup(samples []GoldSampleScore) GoldScoreRollup {
	if len(samples) == 0 {
		return GoldScoreRollup{}
	}
	var summary, drivers, targets, structure, reviewItems float64
	for _, sample := range samples {
		summary += sample.SummaryScore
		drivers += sample.Drivers.Score
		targets += sample.Targets.Score
		structure += sample.StructureScore
		reviewItems += float64(len(sample.ReviewItems))
	}
	n := float64(len(samples))
	return GoldScoreRollup{
		SummaryScore:    roundScore(summary / n),
		DriversScore:    roundScore(drivers / n),
		TargetsScore:    roundScore(targets / n),
		StructureScore:  roundScore(structure / n),
		ReviewItemCount: int(reviewItems),
	}
}

func weightedOverall(summary, drivers, targets, structure float64) float64 {
	return summary*0.25 + drivers*0.3 + targets*0.3 + structure*0.15
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeGoldText(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
