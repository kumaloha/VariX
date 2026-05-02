package verify

import "strings"

const (
	VerificationPassRealized        VerificationPassKind = "realized"
	promptFactVerifierClaim                              = "fact_claim"
	promptFactVerifierChallenge                          = "fact_challenge"
	promptFactVerifierAdjudication                       = "fact_adjudicate"
	promptPredictionVerifier                             = "prediction"
	promptExplicitConditionVerifier                      = "explicit_condition"
	promptImplicitConditionVerifier                      = "implicit_condition"
)

func isConditionVerifierNode(node GraphNode) bool {
	return node.Form == NodeFormCondition ||
		node.Kind == NodeExplicitCondition ||
		node.Kind == NodeImplicitCondition
}

func isImplicitConditionVerifierNode(node GraphNode) bool {
	if node.Kind == NodeImplicitCondition {
		return true
	}
	return node.Form == NodeFormCondition && node.Function != NodeFunctionClaim
}

func classifyVerificationTimeBucket(bundle Bundle, node GraphNode) verificationTimeBucket {
	asOf := verifierNow().UTC()
	if !bundle.PostedAt.IsZero() {
		asOf = bundle.PostedAt.UTC()
	}
	if isConditionVerifierNode(node) {
		return verificationBucketFuture
	}
	if !node.OccurredAt.IsZero() {
		if node.OccurredAt.After(asOf) {
			return verificationBucketFuture
		}
		return verificationBucketRealized
	}
	if !node.PredictionDueAt.IsZero() {
		if node.PredictionDueAt.After(asOf) {
			return verificationBucketFuture
		}
		return verificationBucketRealized
	}
	if !node.PredictionStartAt.IsZero() {
		return verificationBucketFuture
	}
	if !node.ValidFrom.IsZero() && node.ValidFrom.After(asOf) {
		return verificationBucketFuture
	}
	if isObservationLikeVerifierNode(node) && !node.ValidFrom.IsZero() && !node.ValidFrom.After(asOf) {
		return verificationBucketRealized
	}
	if !node.ValidTo.IsZero() && !node.ValidTo.After(asOf) && !node.ValidFrom.IsZero() {
		return verificationBucketRealized
	}
	if looksRealizedText(node.Text) {
		return verificationBucketRealized
	}
	if looksFutureFacingText(node.Text) {
		return verificationBucketFuture
	}
	return verificationBucketUndetermined
}

func looksFutureFacingText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"未来", "将", "会", "预计", "可能", "有望", "下周", "下月", "明年", "季度内", "一旦", "若", "如果",
		"will ", "would ", "may ", "might ", "could ", "expected to", "likely to", "if ", "once ", "approaching",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func looksRealizedText(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"已经", "已", "正在", "当前", "目前", "现已", "进入", "处于", "仍", "仍然", "依然", "并未", "没有形成", "维持", "保持", "存在", "面临", "出现",
		"already", "currently", "remains", "remain", "is in", "are in", "entered", "has entered", "have entered", "is overbought", "has priced in", "have priced in",
		"rally into overbought territory", "overbought territory", "is unresolved", "remains unresolved",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
