package compile

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	salienceChunkRunes = 36000
	salienceMaxChunks  = 6
	salienceOverlap    = 1200
	salienceParallel   = 3
)

func stageSalience(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
	if !shouldRunSalience(bundle, state) {
		return state, nil
	}
	if len(state.SemanticUnits) > 0 {
		state.SemanticUnits = rankSemanticUnits(dedupeSemanticUnits(state.SemanticUnits), state.ArticleForm)
		return state, nil
	}
	units := append([]SemanticUnit(nil), state.SemanticUnits...)
	llmUnits, err := salienceFromLLM(ctx, rt, model, bundle, state)
	if err != nil {
		return graphState{}, err
	}
	units = append(units, llmUnits...)
	state.SemanticUnits = assignSemanticUnitIDs(rankSemanticUnits(dedupeSemanticUnits(units), state.ArticleForm))
	return state, nil
}

func shouldRunSalience(bundle Bundle, state graphState) bool {
	switch strings.ToLower(strings.TrimSpace(state.ArticleForm)) {
	case "management_qa", "shareholder_meeting", "earnings_call", "capital_allocation_discussion":
		return true
	}
	source := strings.ToLower(strings.TrimSpace(bundle.Source))
	return (source == "youtube" || source == "bilibili") && len([]rune(bundle.TextContext())) >= 5000
}

func salienceCategory(unit SemanticUnit) string {
	text := strings.ToLower(strings.Join([]string{unit.ID, unit.Subject, unit.Force, unit.Claim}, " "))
	switch {
	case strings.Contains(text, "capital allocation") || strings.Contains(text, "资本配置"):
		return "capital_allocation"
	case strings.Contains(text, "circle of competence") || strings.Contains(text, "existing portfolio") || strings.Contains(text, "能力圈") || strings.Contains(text, "现有组合"):
		return "portfolio_circle"
	case strings.Contains(text, "cyber") || strings.Contains(text, "网络保险"):
		return "cyber_insurance"
	case strings.Contains(text, "tokyo marine") || strings.Contains(text, "东京海上"):
		return "tokyo_marine"
	case containsAnyText(text, []string{"culture", "values", "current form", "文化", "价值观", "现有形式"}) &&
		!containsAnyText(text, []string{"succession", "继任"}):
		return "culture"
	case containsAnyText(text, []string{"succession", "successor", "继任", "接班"}) &&
		!containsAnyText(text, []string{"culture", "values", "文化", "价值观"}):
		return "succession"
	case strings.Contains(text, "builder of technology") || strings.Contains(text, "technology / ai operating") || strings.Contains(text, "建设技术能力"):
		return "technology_operating_plan"
	default:
		return ""
	}
}

func salienceFromLLM(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) ([]SemanticUnit, error) {
	if rt == nil || strings.TrimSpace(model) == "" {
		return nil, nil
	}
	systemPrompt, err := renderSalienceSystemPrompt()
	if err != nil {
		return nil, err
	}
	chunks := salienceChunks(compactSalienceArticle(bundle.TextContext()))
	if len(chunks) <= 1 {
		return salienceChunkFromLLM(ctx, rt, model, bundle, systemPrompt, state, firstSemanticChunk(chunks), 0, len(chunks))
	}
	results := make([][]SemanticUnit, len(chunks))
	errs := make(chan error, len(chunks))
	sem := make(chan struct{}, salienceParallel)
	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, chunk string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			units, err := salienceChunkFromLLM(ctx, rt, model, bundle, systemPrompt, state, chunk, index, len(chunks))
			if err != nil {
				errs <- err
				return
			}
			results[index] = units
		}(i, chunk)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	units := make([]SemanticUnit, 0)
	for _, chunkUnits := range results {
		units = append(units, chunkUnits...)
	}
	return units, nil
}

func salienceChunkFromLLM(ctx context.Context, rt runtimeChat, model string, bundle Bundle, systemPrompt string, state graphState, chunk string, index int, total int) ([]SemanticUnit, error) {
	userPrompt, err := renderSalienceUserPrompt(chunk, state.ArticleForm, serializeRelationNodes(state.Nodes))
	if err != nil {
		return nil, err
	}
	var result struct {
		SemanticUnits []SemanticUnit `json:"semantic_units"`
	}
	chunkBundle := bundle
	if total > 1 {
		chunkBundle.UnitID = fmt.Sprintf("%s:semantic:%02d", strings.TrimSpace(bundle.UnitID), index+1)
		chunkBundle.Content = chunk
	}
	if err := stageJSONCall(ctx, rt, model, chunkBundle, systemPrompt, userPrompt, "salience", &result); err != nil {
		return nil, err
	}
	for i := range result.SemanticUnits {
		if strings.TrimSpace(result.SemanticUnits[i].Span) == "" && total > 1 {
			result.SemanticUnits[i].Span = fmt.Sprintf("chunk_%02d", index+1)
		}
	}
	return result.SemanticUnits, nil
}

func firstSemanticChunk(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}
	return chunks[0]
}

func salienceChunks(article string) []string {
	article = strings.TrimSpace(article)
	if article == "" {
		return nil
	}
	total := utf8.RuneCountInString(article)
	if total <= salienceChunkRunes {
		return []string{article}
	}
	chunkSize := salienceChunkRunes
	estimated := int(math.Ceil(float64(total) / float64(chunkSize-salienceOverlap)))
	if estimated > salienceMaxChunks {
		chunkSize = int(math.Ceil(float64(total) / float64(salienceMaxChunks)))
		if chunkSize < salienceChunkRunes {
			chunkSize = salienceChunkRunes
		}
	}
	runes := []rune(article)
	chunks := make([]string, 0, salienceMaxChunks)
	for start := 0; start < len(runes); {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) || len(chunks) >= salienceMaxChunks {
			break
		}
		next := end - salienceOverlap
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func compactSalienceArticle(article string) string {
	tokens := strings.Fields(strings.TrimSpace(article))
	if len(tokens) == 0 {
		return ""
	}
	out := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); {
		if skipRepeatedTokenRun(out, tokens, &i) {
			continue
		}
		token := tokens[i]
		if len(out) > 0 && out[len(out)-1] == token {
			i++
			continue
		}
		out = append(out, token)
		i++
	}
	return strings.Join(out, " ")
}

func skipRepeatedTokenRun(out []string, tokens []string, index *int) bool {
	remaining := len(tokens) - *index
	limit := 18
	if len(out) < limit {
		limit = len(out)
	}
	if remaining < limit {
		limit = remaining
	}
	for size := limit; size >= 2; size-- {
		if equalStringWindow(out[len(out)-size:], tokens[*index:*index+size]) {
			*index += size
			return true
		}
	}
	return false
}

func equalStringWindow(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func dedupeSemanticUnits(units []SemanticUnit) []SemanticUnit {
	seen := map[string]int{}
	out := make([]SemanticUnit, 0, len(units))
	for _, unit := range units {
		unit = normalizeSemanticUnit(unit)
		if unit.ID == "" || unit.Subject == "" || unit.Claim == "" {
			continue
		}
		key := strings.ToLower(unit.Subject + "|" + unit.Force)
		if category := salienceCategory(unit); category != "" {
			key = "category:" + category
		}
		if idx, ok := seen[key]; ok {
			out[idx] = mergeSemanticUnit(out[idx], unit)
			continue
		}
		seen[key] = len(out)
		out = append(out, unit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Salience != out[j].Salience {
			return out[i].Salience > out[j].Salience
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func mergeSemanticUnit(base, extra SemanticUnit) SemanticUnit {
	if extra.Salience > base.Salience {
		base, extra = extra, base
	}
	if strings.TrimSpace(base.PromptContext) == "" {
		base.PromptContext = extra.PromptContext
	}
	if strings.TrimSpace(base.ImportanceReason) == "" {
		base.ImportanceReason = extra.ImportanceReason
	}
	if strings.TrimSpace(base.SourceQuote) == "" {
		base.SourceQuote = extra.SourceQuote
	}
	if strings.TrimSpace(base.Speaker) == "" {
		base.Speaker = extra.Speaker
	}
	return base
}

func rankSemanticUnits(units []SemanticUnit, _ string) []SemanticUnit {
	if len(units) == 0 {
		return nil
	}
	out := append([]SemanticUnit(nil), units...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Salience != out[j].Salience {
			return out[i].Salience > out[j].Salience
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func assignSemanticUnitIDs(units []SemanticUnit) []SemanticUnit {
	if len(units) == 0 {
		return nil
	}
	out := append([]SemanticUnit(nil), units...)
	for i := range out {
		out[i].ID = fmt.Sprintf("semantic-%03d", i+1)
	}
	return out
}

func topSemanticUnitsForMainline(units []SemanticUnit, articleForm string) []SemanticUnit {
	if len(units) == 0 {
		return nil
	}
	ranked := rankSemanticUnits(units, articleForm)
	if isReaderInterestSummaryForm(articleForm) {
		sortSemanticUnitsForReaderInterest(ranked)
	}
	limit := semanticMainlineUnitLimit(articleForm)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func semanticMainlineUnitLimit(articleForm string) int {
	switch strings.ToLower(strings.TrimSpace(articleForm)) {
	case "management_qa", "shareholder_meeting", "earnings_call", "capital_allocation_discussion":
		return 18
	default:
		return 8
	}
}

func normalizeSemanticUnit(unit SemanticUnit) SemanticUnit {
	unit.ID = strings.TrimSpace(unit.ID)
	unit.Span = strings.TrimSpace(unit.Span)
	unit.Speaker = strings.TrimSpace(unit.Speaker)
	unit.SpeakerRole = normalizeSemanticSpeakerRole(unit.SpeakerRole)
	unit.Subject = strings.TrimSpace(unit.Subject)
	unit.Force = normalizeSemanticForce(unit.Force)
	unit.Claim = strings.TrimSpace(unit.Claim)
	unit.PromptContext = strings.TrimSpace(unit.PromptContext)
	unit.ImportanceReason = strings.TrimSpace(unit.ImportanceReason)
	unit.SourceQuote = strings.TrimSpace(unit.SourceQuote)
	unit.Confidence = strings.TrimSpace(unit.Confidence)
	return unit
}

func normalizeSemanticSpeakerRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "primary", "named_participant", "questioner":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func normalizeSemanticForce(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "assert", "report", "explain", "answer", "commit", "reject", "disclose", "frame_risk", "set_boundary":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "assert"
	}
}

func serializeSemanticUnitsForMainline(units []SemanticUnit) string {
	if len(units) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for _, unit := range units {
		fmt.Fprintf(&b, "- %s [%s/%s] %s: %s\n", unit.ID, unit.SpeakerRole, unit.Force, unit.Subject, unit.Claim)
		if context := strings.TrimSpace(unit.PromptContext); context != "" {
			fmt.Fprintf(&b, "  prompt_context: %s\n", context)
		}
		if quote := strings.TrimSpace(unit.SourceQuote); quote != "" {
			fmt.Fprintf(&b, "  source_quote: %s\n", quote)
		}
	}
	return strings.TrimSpace(b.String())
}

func renderSemanticUnitDetails(units []SemanticUnit) []map[string]any {
	if len(units) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(units))
	for _, unit := range units {
		item := map[string]any{
			"kind":    "semantic_unit",
			"id":      unit.ID,
			"subject": unit.Subject,
			"claim":   unit.Claim,
		}
		if speaker := strings.TrimSpace(unit.Speaker); speaker != "" {
			item["speaker"] = speaker
		}
		if force := strings.TrimSpace(unit.Force); force != "" {
			item["force"] = force
		}
		if quote := strings.TrimSpace(unit.SourceQuote); quote != "" {
			item["source_quote"] = quote
		}
		out = append(out, item)
	}
	return out
}

func prioritizeSemanticSummary(summary string, units []SemanticUnit, articleForm string) string {
	summary = strings.TrimSpace(summary)
	if len(units) == 0 {
		return summary
	}
	readerInterest := isReaderInterestSummaryForm(articleForm)
	orderedUnits := units
	if readerInterest {
		orderedUnits = topSemanticUnitsForSummary(units, articleForm)
	}
	extras := make([]string, 0, 2)
	for _, unit := range orderedUnits {
		if len(extras) >= 2 {
			break
		}
		if unit.Salience < 0.75 {
			continue
		}
		if salienceCategory(unit) == "capital_allocation" {
			continue
		}
		if readerInterest && summaryReaderInterestRank(unit) >= 8 {
			continue
		}
		claim := semanticSummaryClaim(unit)
		if claim == "" || semanticSummaryAlreadyCovers(summary, claim, unit.Subject) {
			continue
		}
		extras = append(extras, claim)
	}
	if len(extras) == 0 {
		return summary
	}
	if summary == "" {
		return strings.Join(extras, "；") + "。"
	}
	return strings.TrimRight(summary, "。.!！") + "；" + strings.Join(extras, "；") + "。"
}

func semanticSummaryClaim(unit SemanticUnit) string {
	claim := strings.TrimSpace(unit.Claim)
	claim = trimSemanticSummarySentence(claim)
	if before, _, ok := strings.Cut(claim, "。"); ok {
		if len([]rune(before)) >= 24 {
			claim = before
		}
	}
	return truncateRunes(claim, 120)
}

func semanticSummaryAlreadyCovers(summary, claim, subject string) bool {
	lowerSummary := strings.ToLower(summary)
	for _, marker := range []string{subject, firstSemanticKeyword(claim)} {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker != "" && strings.Contains(lowerSummary, marker) {
			return true
		}
	}
	return false
}

func firstSemanticKeyword(claim string) string {
	for _, marker := range []string{"Apple", "AI", "technology", "技术", "科技股", "消费者", "GEICO", "BNSF"} {
		if strings.Contains(strings.ToLower(claim), strings.ToLower(marker)) {
			return marker
		}
	}
	return ""
}

func trimSemanticSummarySentence(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, "。.!！")
	return truncateRunes(value, 180)
}
