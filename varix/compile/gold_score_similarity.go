package compile

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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
