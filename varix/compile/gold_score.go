package compile

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const goldMatchThreshold = 0.38

type goldConcept struct {
	Token   string
	Weight  int
	Aliases []string
}

var goldFinanceConcepts = []goldConcept{
	{Token: "ai", Weight: 2, Aliases: []string{"ai", "artificial intelligence", "人工智能"}},
	{Token: "asset", Weight: 1, Aliases: []string{"asset", "assets", "资产"}},
	{Token: "basel", Weight: 2, Aliases: []string{"basel", "巴塞尔"}},
	{Token: "bitcoin", Weight: 2, Aliases: []string{"bitcoin", "btc", "比特币"}},
	{Token: "bond", Weight: 2, Aliases: []string{"bond", "bonds", "主权债", "国债", "债券"}},
	{Token: "capital", Weight: 1, Aliases: []string{"capital", "fund", "funds", "money", "资金", "资本"}},
	{Token: "cash", Weight: 2, Aliases: []string{"cash", "现金"}},
	{Token: "central_bank", Weight: 2, Aliases: []string{"central bank", "fed", "federal reserve", "央行", "美联储"}},
	{Token: "credit", Weight: 2, Aliases: []string{"credit", "信贷", "信用"}},
	{Token: "debt", Weight: 2, Aliases: []string{"debt", "债务"}},
	{Token: "dollar", Weight: 2, Aliases: []string{"dollar", "usd", "美元"}},
	{Token: "flow", Weight: 2, Aliases: []string{"flow", "flows", "inflow", "inflows", "outflow", "outflows", "reallocate", "reallocation", "rotation", "流入", "流出", "回流", "配置", "转向"}},
	{Token: "foreign", Weight: 2, Aliases: []string{"foreign", "overseas", "global", "海外", "全球"}},
	{Token: "gold", Weight: 2, Aliases: []string{"gold", "黄金"}},
	{Token: "growth", Weight: 2, Aliases: []string{"growth", "增长"}},
	{Token: "gsib", Weight: 2, Aliases: []string{"gsib", "g-sib"}},
	{Token: "hard_asset", Weight: 2, Aliases: []string{"hard asset", "hard assets", "tangible asset", "tangible assets", "real asset", "real assets", "实物资产", "硬资产"}},
	{Token: "inflation", Weight: 2, Aliases: []string{"inflation", "通胀"}},
	{Token: "infrastructure", Weight: 2, Aliases: []string{"infrastructure", "capex", "基建", "资本开支"}},
	{Token: "interest_rate", Weight: 2, Aliases: []string{"interest rate", "interest rates", "rates", "yield", "yields", "利率", "收益率"}},
	{Token: "iran_war", Weight: 2, Aliases: []string{"iran conflict", "iran war", "伊朗战争", "伊朗战事"}},
	{Token: "liquidity", Weight: 2, Aliases: []string{"liquidity", "流动性"}},
	{Token: "loan", Weight: 2, Aliases: []string{"loan", "loans", "lending", "放贷", "贷款"}},
	{Token: "long_rate", Weight: 2, Aliases: []string{"long-term rate", "long-term rates", "long term rate", "long term rates", "长端利率"}},
	{Token: "narrative", Weight: 2, Aliases: []string{"narrative", "exceptionalism", "叙事", "例外论"}},
	{Token: "oil", Weight: 2, Aliases: []string{"oil", "petrodollar", "石油", "油价", "石油美元"}},
	{Token: "political_risk", Weight: 2, Aliases: []string{"political risk", "politicized", "政治风险", "政治化"}},
	{Token: "private_credit", Weight: 3, Aliases: []string{"private credit", "private-credit", "私募信贷"}},
	{Token: "real_yield", Weight: 3, Aliases: []string{"real yield", "real yields", "real return", "real returns", "实际收益率", "真实收益率", "负真实收益率"}},
	{Token: "redemption", Weight: 2, Aliases: []string{"redemption", "redemptions", "赎回"}},
	{Token: "regulation", Weight: 2, Aliases: []string{"regulation", "regulatory", "监管"}},
	{Token: "reserve", Weight: 2, Aliases: []string{"reserve", "reserves", "准备金"}},
	{Token: "risk", Weight: 1, Aliases: []string{"risk", "risks", "风险"}},
	{Token: "safe_haven", Weight: 2, Aliases: []string{"safe haven", "safe-haven", "避险"}},
	{Token: "supply_chain", Weight: 2, Aliases: []string{"supply chain", "供应链", "产业链"}},
	{Token: "us", Weight: 1, Aliases: []string{"u.s.", "us", "usa", "america", "american", "美国"}},
}

type GoldCandidate struct {
	SampleID string `json:"sample_id"`
	Output   Output `json:"output"`
}

type GoldScorecard struct {
	DatasetVersion string            `json:"dataset_version"`
	SampleCount    int               `json:"sample_count"`
	OverallScore   float64           `json:"overall_score"`
	Rollup         GoldScoreRollup   `json:"rollup"`
	Samples        []GoldSampleScore `json:"samples"`
}

type GoldScoreRollup struct {
	SummaryScore    float64 `json:"summary_score"`
	DriversScore    float64 `json:"drivers_score"`
	TargetsScore    float64 `json:"targets_score"`
	StructureScore  float64 `json:"structure_score"`
	ReviewItemCount int     `json:"review_item_count"`
}

type GoldSampleScore struct {
	ID             string           `json:"id"`
	Title          string           `json:"title,omitempty"`
	OverallScore   float64          `json:"overall_score"`
	SummaryScore   float64          `json:"summary_score"`
	Drivers        GoldListScore    `json:"drivers"`
	Targets        GoldListScore    `json:"targets"`
	StructureScore float64          `json:"structure_score"`
	ReviewItems    []GoldReviewItem `json:"review_items,omitempty"`
}

type GoldListScore struct {
	Score          float64     `json:"score"`
	Precision      float64     `json:"precision"`
	Recall         float64     `json:"recall"`
	F1             float64     `json:"f1"`
	GoldCount      int         `json:"gold_count"`
	CandidateCount int         `json:"candidate_count"`
	MatchedCount   int         `json:"matched_count"`
	MissingCount   int         `json:"missing_count"`
	ExtraCount     int         `json:"extra_count"`
	Matches        []GoldMatch `json:"matches,omitempty"`
	MissingGold    []string    `json:"missing_gold,omitempty"`
	ExtraCandidate []string    `json:"extra_candidate,omitempty"`
}

type GoldMatch struct {
	Gold       string  `json:"gold"`
	Candidate  string  `json:"candidate"`
	Similarity float64 `json:"similarity"`
}

type GoldReviewItem struct {
	Severity  string  `json:"severity"`
	Field     string  `json:"field"`
	Kind      string  `json:"kind"`
	Score     float64 `json:"score,omitempty"`
	Gold      string  `json:"gold,omitempty"`
	Candidate string  `json:"candidate,omitempty"`
	Message   string  `json:"message"`
}

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

func textSimilarity(a, b string) float64 {
	tokenScore := diceCount(goldSemanticTokens(a), goldSemanticTokens(b))
	a = normalizeGoldText(a)
	b = normalizeGoldText(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return 0.92
	}
	bigramScore := diceCount(runeBigrams(a), runeBigrams(b))
	if tokenScore > bigramScore {
		return tokenScore
	}
	return bigramScore
}

func goldSemanticTokens(text string) map[string]int {
	out := goldTokens(text)
	lower := strings.ToLower(text)
	compacted := normalizeGoldText(text)
	for _, concept := range goldFinanceConcepts {
		for _, alias := range concept.Aliases {
			if goldConceptAliasPresent(lower, compacted, alias) {
				out["concept:"+concept.Token] += concept.Weight
				break
			}
		}
	}
	return out
}

func goldConceptAliasPresent(lower, compacted, alias string) bool {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" {
		return false
	}
	if strings.Contains(lower, alias) {
		return true
	}
	return strings.Contains(compacted, normalizeGoldText(alias))
}

func normalizeGoldText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			continue
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func goldTokens(text string) map[string]int {
	out := map[string]int{}
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}) {
		token = strings.TrimSpace(token)
		if token != "" {
			out[token]++
		}
	}
	return out
}

func runeBigrams(text string) map[string]int {
	out := map[string]int{}
	if utf8.RuneCountInString(text) < 2 {
		if text != "" {
			out[text] = 1
		}
		return out
	}
	runes := []rune(text)
	for i := 0; i < len(runes)-1; i++ {
		out[string(runes[i:i+2])]++
	}
	return out
}

func diceCount(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	total := 0
	for key, countA := range a {
		total += countA
		if countB, ok := b[key]; ok {
			intersection += minGoldInt(countA, countB)
		}
	}
	for _, countB := range b {
		total += countB
	}
	if total == 0 {
		return 0
	}
	return float64(2*intersection) / float64(total)
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

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		if numerator == 0 {
			return 1
		}
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func roundScore(value float64) float64 {
	return math.Round(value*100) / 100
}

func roundRatio(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func severityForScore(score float64) string {
	if score < 40 {
		return "high"
	}
	return "medium"
}

func minGoldInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
