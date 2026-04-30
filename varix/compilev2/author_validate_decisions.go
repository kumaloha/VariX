package compilev2

import (
	"github.com/kumaloha/VariX/varix/compile"
	"regexp"
	"strconv"
	"strings"
)

func authorClaimComparableNumbers(check compile.AuthorClaimCheck) []authorComparableNumber {
	parts := []string{check.Text}
	for _, requirement := range check.RequiredEvidence {
		parts = append(parts, requirement.OriginalValue, requirement.Description, requirement.Reason)
	}
	return parseAuthorComparableNumbers(strings.Join(parts, " "))
}

func parseAuthorComparableNumbers(text string) []authorComparableNumber {
	pattern := regexp.MustCompile(`(?i)(减少|下降|decrease(?:d)?|decline(?:d)?|drop(?:ped)?|down|[<>])?\s*(-?\d+(?:\.\d+)?)\s*(万亿|亿美元|亿美金|trillion|billion|t|b|万亿美金|万亿美元)`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	out := make([]authorComparableNumber, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		comparator := strings.TrimSpace(match[1])
		if value > 0 && isDecreaseMarker(comparator) {
			value = -value
			comparator = ""
		}
		unit := strings.ToLower(match[3])
		switch unit {
		case "万亿", "万亿美金", "万亿美元", "trillion", "t":
			unit = "trillion"
		case "亿美元", "亿美金":
			value = value / 10
			unit = "billion"
		case "billion", "b":
			unit = "billion"
		}
		key := comparator + "|" + strconv.FormatFloat(value, 'f', 6, 64) + "|" + unit
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, authorComparableNumber{Value: value, Unit: unit, Comparator: comparator})
	}
	return out
}

func isDecreaseMarker(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "减少", "下降", "decrease", "decreased", "decline", "declined", "drop", "dropped", "down":
		return true
	default:
		return false
	}
}

func externalEvidenceNumbersSupport(authorNumbers []authorComparableNumber, result authorExternalEvidenceResult) bool {
	sourceValues := externalEvidenceComparableValues(result)
	if len(sourceValues) == 0 {
		return false
	}
	for _, authorNumber := range authorNumbers {
		if !anySourceValueMatchesAuthorNumber(sourceValues, authorNumber) {
			return false
		}
	}
	return true
}

func externalEvidenceComparableValues(result authorExternalEvidenceResult) []float64 {
	text := strings.Join([]string{result.Title, result.Excerpt}, " ")
	rawMatches := regexp.MustCompile(`=\s*(-?\d+(?:\.\d+)?)`).FindAllStringSubmatch(text, -1)
	out := make([]float64, 0, len(rawMatches))
	isStablecoin := strings.Contains(strings.ToLower(result.Title+" "+result.URL), "stablecoin") || strings.Contains(strings.ToLower(result.URL), "stablecoins")
	for _, match := range rawMatches {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(strings.ToUpper(result.Title), "FRED WRESBAL"), strings.Contains(strings.ToUpper(result.Title), "FRED WALCL"):
			value = value / 1_000_000
		case isStablecoin:
			value = value / 1_000_000_000
		}
		out = append(out, value)
	}
	return out
}

func anySourceValueMatchesAuthorNumber(sourceValues []float64, authorNumber authorComparableNumber) bool {
	for _, sourceValue := range sourceValues {
		switch authorNumber.Comparator {
		case "<":
			if sourceValue < authorNumber.Value*1.02 {
				return true
			}
		case ">":
			if sourceValue > authorNumber.Value*0.98 {
				return true
			}
		default:
			tolerance := 0.08
			if authorNumber.Value >= 5 {
				tolerance = 0.05
			}
			if authorNumber.Value != 0 && absFloat64(sourceValue-authorNumber.Value)/absFloat64(authorNumber.Value) <= tolerance {
				return true
			}
		}
	}
	return false
}

func absFloat64(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func exactExternalEvidenceHintForClaim(check compile.AuthorClaimCheck, hints []authorExternalEvidenceHint) (authorExternalEvidenceResult, bool) {
	claimText := strings.ToLower(strings.TrimSpace(check.Text + " " + check.Reason))
	for _, requirement := range check.RequiredEvidence {
		claimText += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.Subject,
			requirement.Metric,
			requirement.TimeWindow,
			requirement.SourceType,
			requirement.Reason,
		}, " "))
	}
	wantsEIAOil91 := (strings.Contains(claimText, "910") || strings.Contains(claimText, "9.1")) &&
		(strings.Contains(claimText, "oil") || strings.Contains(claimText, "石油") || strings.Contains(claimText, "production") || strings.Contains(claimText, "减产")) &&
		(strings.Contains(claimText, "eia") || strings.Contains(claimText, "energy information") || strings.Contains(claimText, "能源信息署") || strings.Contains(claimText, "official"))
	for _, hint := range hints {
		for _, result := range hint.Results {
			evidenceText := strings.ToLower(strings.Join([]string{hint.Query, result.URL, result.Title, result.Excerpt}, " "))
			if wantsEIAOil91 &&
				strings.Contains(evidenceText, "eia") &&
				strings.Contains(evidenceText, "9.1 million b/d") &&
				strings.Contains(evidenceText, "production shut-ins") &&
				strings.Contains(evidenceText, "april") {
				return result, true
			}
		}
	}
	return authorExternalEvidenceResult{}, false
}

func enforceExternalEvidenceForSupportedClaim(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	if check.Status != compile.AuthorClaimSupported || hasExternalClaimSupport(check) {
		return check
	}
	check.Status = compile.AuthorClaimUnverified
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimSupported && !requirementHasExternalSupport(check.RequiredEvidence[i]) {
			check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
		}
	}
	check.Reason = appendAuthorValidationReason(check.Reason, "Supported checkable claims require external evidence; the returned evidence only restates or cites the author.")
	return check
}

func enforceLegalClaimScope(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	if check.Status != compile.AuthorClaimSupported || !legalClaimNeedsSpecificAllegationSupport(check) {
		return check
	}
	externalText := strings.ToLower(strings.Join(externalEvidenceStringsForClaim(check), " "))
	if externalText == "" || legalEvidenceSupportsSpecificMethod(externalText) {
		return check
	}
	check.Status = compile.AuthorClaimUnverified
	for i := range check.RequiredEvidence {
		check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
		check.RequiredEvidence[i].Reason = "External legal evidence confirms lawsuit/allegation existence but does not verify the specific trading-method detail."
	}
	check.Reason = "External legal evidence confirms lawsuit/allegation existence but does not verify the specific trading-method detail."
	return check
}

func fillAuthorClaimDecisionNote(check compile.AuthorClaimCheck) compile.AuthorClaimCheck {
	for i := range check.Subclaims {
		check.Subclaims[i].DecisionNote = authorSubclaimDecisionNote(check.Subclaims[i])
	}
	check.DecisionNote = authorClaimDecisionNote(check)
	return check
}

func authorClaimDecisionNote(check compile.AuthorClaimCheck) string {
	basis := authorClaimBasisSummary(check)
	reason := authorClaimReasonSummary(check)
	parts := make([]string, 0, 2)
	if basis != "" {
		parts = append(parts, "口径: "+basis)
	}
	parts = append(parts, "判定: "+firstNonEmpty(reason, string(check.Status)))
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func authorClaimBasisSummary(check compile.AuthorClaimCheck) string {
	for _, requirement := range check.RequiredEvidence {
		if basis := authorRequirementBasisSummary(requirement); basis != "" {
			return basis
		}
	}
	for _, subclaim := range check.Subclaims {
		parts := make([]string, 0, 5)
		if subject := strings.TrimSpace(subclaim.Subject); subject != "" {
			parts = append(parts, subject)
		}
		if metric := strings.TrimSpace(subclaim.Metric); metric != "" {
			parts = append(parts, metric)
		}
		if value := strings.TrimSpace(subclaim.OriginalValue); value != "" {
			parts = append(parts, "作者值 "+value)
		}
		if base := strings.TrimSpace(subclaim.ComparisonBase); base != "" {
			parts = append(parts, "分母/对象 "+base)
		}
		if evidenceBase := strings.TrimSpace(subclaim.EvidenceBase); evidenceBase != "" {
			parts = append(parts, "证据对象 "+evidenceBase)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "；")
		}
	}
	return ""
}

func authorRequirementBasisSummary(requirement compile.AuthorEvidenceRequirement) string {
	parts := make([]string, 0, 8)
	if subject := strings.TrimSpace(requirement.Subject); subject != "" {
		parts = append(parts, subject)
	}
	if metric := strings.TrimSpace(requirement.Metric); metric != "" {
		parts = append(parts, metric)
	}
	if value := strings.TrimSpace(requirement.OriginalValue); value != "" {
		parts = append(parts, "作者值 "+value)
	}
	if unit := strings.TrimSpace(requirement.Unit); unit != "" {
		parts = append(parts, "单位 "+unit)
	}
	if window := strings.TrimSpace(requirement.TimeWindow); window != "" {
		parts = append(parts, "窗口 "+window)
	}
	if source := firstNonEmpty(requirement.Series, requirement.SourceType, firstString(requirement.PreferredSources)); source != "" {
		parts = append(parts, "来源 "+source)
	}
	if denominator := strings.TrimSpace(requirement.Denominator); denominator != "" {
		parts = append(parts, "分母 "+denominator)
	}
	if caveat := strings.TrimSpace(requirement.ScopeCaveat); caveat != "" {
		parts = append(parts, "范围 "+caveat)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "；")
}

func authorClaimReasonSummary(check compile.AuthorClaimCheck) string {
	if reason := strings.TrimSpace(check.Reason); reason != "" {
		return reason
	}
	for _, subclaim := range check.Subclaims {
		if subclaim.Status == check.Status {
			if reason := strings.TrimSpace(subclaim.Reason); reason != "" {
				return reason
			}
		}
	}
	for _, requirement := range check.RequiredEvidence {
		if requirement.Status == check.Status {
			if reason := strings.TrimSpace(requirement.Reason); reason != "" {
				return reason
			}
		}
	}
	for _, requirement := range check.RequiredEvidence {
		if reason := strings.TrimSpace(requirement.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

func authorSubclaimDecisionNote(subclaim compile.AuthorSubclaim) string {
	basisParts := make([]string, 0, 6)
	if subject := strings.TrimSpace(subclaim.Subject); subject != "" {
		basisParts = append(basisParts, subject)
	}
	if metric := strings.TrimSpace(subclaim.Metric); metric != "" {
		basisParts = append(basisParts, metric)
	}
	if value := strings.TrimSpace(subclaim.OriginalValue); value != "" {
		basisParts = append(basisParts, "作者值 "+value)
	}
	if evidenceValue := strings.TrimSpace(firstNonEmpty(subclaim.EvidenceValue, subclaim.EvidenceRange)); evidenceValue != "" {
		basisParts = append(basisParts, "证据值 "+evidenceValue)
	}
	if scope := strings.TrimSpace(subclaim.ScopeStatus); scope != "" {
		basisParts = append(basisParts, "范围 "+scope)
	}
	parts := make([]string, 0, 2)
	if len(basisParts) > 0 {
		parts = append(parts, "口径: "+strings.Join(basisParts, "；"))
	}
	parts = append(parts, "判定: "+firstNonEmpty(subclaim.Reason, string(subclaim.Status)))
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func fillAuthorInferenceDecisionNote(check compile.AuthorInferenceCheck) compile.AuthorInferenceCheck {
	check.DecisionNote = authorInferenceDecisionNote(check)
	return check
}

func authorInferenceDecisionNote(check compile.AuthorInferenceCheck) string {
	basisParts := make([]string, 0, 2)
	path := authorInferencePathText(check)
	if path != "" {
		basisParts = append(basisParts, "路径 "+path)
	}
	for _, requirement := range check.RequiredEvidence {
		if basis := authorRequirementBasisSummary(requirement); basis != "" {
			basisParts = append(basisParts, "所需证据 "+basis)
			break
		}
	}
	parts := make([]string, 0, 2)
	if len(basisParts) > 0 {
		parts = append(parts, "口径: "+strings.Join(basisParts, "；"))
	}
	parts = append(parts, "判定: "+firstNonEmpty(check.Reason, string(check.Status)))
	if len(check.MissingLinks) > 0 {
		parts = append(parts, "缺口: "+strings.Join(check.MissingLinks, "；"))
	}
	return truncateAuthorEvidenceExcerpt(strings.Join(parts, "；"), 480)
}

func authorInferencePathText(check compile.AuthorInferenceCheck) string {
	parts := make([]string, 0, len(check.Steps)+2)
	if from := strings.TrimSpace(check.From); from != "" {
		parts = append(parts, from)
	}
	parts = append(parts, trimmedStringSlice(check.Steps)...)
	if to := strings.TrimSpace(check.To); to != "" {
		parts = append(parts, to)
	}
	return strings.Join(parts, " -> ")
}

func legalClaimNeedsSpecificAllegationSupport(check compile.AuthorClaimCheck) bool {
	text := strings.ToLower(check.Text + " " + check.Reason)
	for _, requirement := range check.RequiredEvidence {
		text += " " + strings.ToLower(strings.Join([]string{
			requirement.Description,
			requirement.Subject,
			requirement.Metric,
			requirement.OriginalValue,
			requirement.SourceType,
			requirement.ComparisonRule,
			requirement.ScopeCaveat,
			strings.Join(requirement.PreferredSources, " "),
		}, " "))
	}
	hasLegalSubject := containsAny(text, "lawsuit", "class action", "courtlistener", "pacer", "legal", "诉讼", "集体诉讼", "jane street")
	hasSpecificMethod := containsAny(text,
		"timed selling",
		"daily",
		"large sell",
		"sell order",
		"liquidation",
		"forced liquidation",
		"buy back",
		"low-level buying",
		"每日",
		"定时",
		"大额抛售",
		"抛售",
		"爆仓",
		"低位",
		"补仓",
	)
	return hasLegalSubject && hasSpecificMethod
}

func legalEvidenceSupportsSpecificMethod(externalText string) bool {
	return containsAny(externalText,
		"timed selling",
		"daily sell",
		"large sell",
		"sell order",
		"liquidation",
		"forced liquidation",
		"buy back",
		"low-level buying",
		"每日",
		"定时",
		"大额抛售",
		"爆仓",
		"低位补仓",
	)
}

func externalEvidenceStringsForClaim(check compile.AuthorClaimCheck) []string {
	out := make([]string, 0, len(check.Evidence)+len(check.RequiredEvidence))
	for _, evidence := range check.Evidence {
		if isExternalEvidenceString(evidence) {
			out = append(out, evidence)
		}
	}
	for _, requirement := range check.RequiredEvidence {
		for _, evidence := range requirement.Evidence {
			if isExternalEvidenceString(evidence) {
				out = append(out, evidence)
			}
		}
	}
	return out
}

func enforceExternalEvidenceForSoundInference(check compile.AuthorInferenceCheck) compile.AuthorInferenceCheck {
	if check.Status != compile.AuthorInferenceSound || hasExternalInferenceSupport(check) {
		return check
	}
	check.Status = compile.AuthorInferenceWeak
	for i := range check.RequiredEvidence {
		if check.RequiredEvidence[i].Status == compile.AuthorClaimSupported && !requirementHasExternalSupport(check.RequiredEvidence[i]) {
			check.RequiredEvidence[i].Status = compile.AuthorClaimUnverified
			check.RequiredEvidence[i].Reason = "Sound inference requires external support for the necessary factual premise, not only author provenance."
		}
	}
	check.Reason = "Sound inference requires external support for the necessary factual premises, not only author provenance."
	check.MissingLinks = appendAuthorValidationUniqueString(check.MissingLinks, "external evidence for the factual premises needed by this inference")
	return check
}

func hasExternalClaimSupport(check compile.AuthorClaimCheck) bool {
	if hasExternalEvidenceStrings(check.Evidence) {
		return true
	}
	for _, requirement := range check.RequiredEvidence {
		if requirementHasExternalSupport(requirement) {
			return true
		}
	}
	for _, subclaim := range check.Subclaims {
		if subclaim.Status != compile.AuthorClaimSupported {
			continue
		}
		if hasExternalEvidenceStrings(subclaim.Evidence) ||
			strings.TrimSpace(subclaim.EvidenceValue) != "" ||
			strings.TrimSpace(subclaim.EvidenceRange) != "" {
			return true
		}
	}
	return false
}

func hasExternalInferenceSupport(check compile.AuthorInferenceCheck) bool {
	if hasExternalEvidenceStrings(check.Evidence) {
		return true
	}
	for _, requirement := range check.RequiredEvidence {
		if requirementHasExternalSupport(requirement) {
			return true
		}
	}
	return false
}

func requirementHasExternalSupport(requirement compile.AuthorEvidenceRequirement) bool {
	if requirement.Status != compile.AuthorClaimSupported {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(requirement.SourceType), "author_source") {
		return false
	}
	return hasExternalEvidenceStrings(requirement.Evidence)
}

func hasExternalEvidenceStrings(values []string) bool {
	for _, value := range values {
		if isExternalEvidenceString(value) {
			return true
		}
	}
	return false
}

func isExternalEvidenceString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if isAuthorOnlyEvidenceString(normalized) {
		return false
	}
	if isVagueExternalEvidenceString(normalized) {
		return false
	}
	externalMarkers := []string{
		"http://",
		"https://",
		"www.",
		".gov",
		"source says",
		"official source",
		"official data",
		"official report",
		"official release",
		"central bank release",
		"company filing",
		"market data",
		"reports",
		"report:",
		"release:",
		"filing:",
		"eia ",
		"eia:",
		"steo",
		"iea ",
		"iea:",
		"fred",
		"defillama",
		"sosovalue",
		"courtlistener",
		"pacer",
		"treasury",
		"world gold council",
		"wgc",
		"bloomberg",
		"reuters",
		"s&p global",
		"federal reserve",
		"cbo",
		"bea",
		"bls",
		"sec ",
		"sec:",
	}
	for _, marker := range externalMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isVagueExternalEvidenceString(normalized string) bool {
	vagueMarkers := []string{
		"industry reports",
		"market data shows",
		"data shows",
		"reports show",
		"research shows",
		"often",
		"typically",
		"e.g.",
		"for example",
		"generally",
	}
	hasVagueMarker := false
	for _, marker := range vagueMarkers {
		if strings.Contains(normalized, marker) {
			hasVagueMarker = true
			break
		}
	}
	if !hasVagueMarker {
		return false
	}
	concreteMarkers := []string{
		"http://",
		"https://",
		".gov",
		"fred:",
		"fred ",
		"steo",
		"courtlistener",
		"pacer",
	}
	for _, marker := range concreteMarkers {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	hasNumber := regexp.MustCompile(`\d`).MatchString(normalized)
	hasNamedDatedSource := regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec|20\d{2}|q[1-4])`).MatchString(normalized)
	if hasNumber && hasNamedDatedSource {
		return false
	}
	return true
}
