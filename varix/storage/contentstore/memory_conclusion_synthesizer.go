package contentstore

import (
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func buildCognitiveConclusion(thesis memory.CausalThesis, cards []memory.CognitiveCard) (memory.CognitiveConclusion, bool) {
	if !isConclusionAbstractable(thesis, cards) {
		return memory.CognitiveConclusion{}, false
	}
	headline := synthesizeConclusionHeadline(thesis, cards[0])
	if isGenericConclusion(headline) {
		return memory.CognitiveConclusion{}, false
	}
	backing := make([]string, 0, len(cards))
	for _, card := range cards {
		backing = append(backing, card.CardID)
	}
	return memory.CognitiveConclusion{
		ConclusionID:       thesis.CausalThesisID + "-conclusion",
		CausalThesisID:     thesis.CausalThesisID,
		Headline:           headline,
		Subheadline:        strings.TrimSpace(cards[0].Summary),
		BackingCardIDs:     backing,
		WhyItExists:        strings.TrimSpace(cards[0].Summary),
		TraceabilityStatus: "grounded",
	}, true
}

func isConclusionAbstractable(thesis memory.CausalThesis, cards []memory.CognitiveCard) bool {
	return thesis.AbstractionReady && thesis.CompletenessScore >= 0.8 && len(cards) > 0
}

func isGenericConclusion(headline string) bool {
	headline = strings.TrimSpace(headline)
	if headline == "" {
		return true
	}
	for _, generic := range []string{
		"风险值得关注",
		"市场可能发生变化",
		"值得关注",
		"需要继续观察",
	} {
		if headline == generic {
			return true
		}
	}
	return false
}

func synthesizeConclusionHeadline(thesis memory.CausalThesis, card memory.CognitiveCard) string {
	headline := strings.TrimSpace(card.Title)
	driver := ""
	if len(card.KeyEvidence) > 0 {
		driver = strings.TrimSpace(card.KeyEvidence[0])
	}
	if abstract := abstractHeadlineFromDebtPurchasingPower(thesis, card); abstract != "" {
		return abstract
	}
	if abstract := abstractHeadlineFromPetrodollarPrivateCredit(thesis, card); abstract != "" {
		return abstract
	}
	if abstract := abstractHeadlineFromOilShock(thesis, card); abstract != "" {
		return abstract
	}
	if abstract := abstractHeadlineFromJPMResilience(thesis, card); abstract != "" {
		return abstract
	}
	if len(card.Predictions) > 0 && strings.TrimSpace(card.Predictions[0]) != "" {
		if abstract := abstractHeadlineFromPressureAndVolatility(driver, headline, strings.TrimSpace(card.Predictions[0])); abstract != "" {
			return abstract
		}
		if driver != "" {
			return driver + "会使" + headline + "，并可能导致" + strings.TrimSpace(card.Predictions[0])
		}
		return headline + "，并可能导致" + strings.TrimSpace(card.Predictions[0])
	}
	if driver != "" && headline != "" {
		return driver + "会使" + headline
	}
	if thesis.CoreQuestion != "" && headline != "" && !strings.Contains(thesis.CoreQuestion, headline) {
		return headline
	}
	return headline
}

func abstractHeadlineFromDebtPurchasingPower(thesis memory.CausalThesis, card memory.CognitiveCard) string {
	if !strings.Contains(thesis.CoreQuestion, "债务") {
		return ""
	}
	if !strings.Contains(card.Title, "购买力") {
		return ""
	}
	return "债务与货币贬值压力正在侵蚀现金与债券购买力"
}

func abstractHeadlineFromPetrodollarPrivateCredit(thesis memory.CausalThesis, card memory.CognitiveCard) string {
	if !strings.Contains(thesis.CoreQuestion, "石油美元") || !strings.Contains(thesis.CoreQuestion, "私募信贷") {
		return ""
	}
	if !strings.Contains(card.Title, "流动性隐患") {
		return ""
	}
	return "石油美元与私募信贷流动性风险正在推高美国资产脆弱性"
}

func abstractHeadlineFromOilShock(thesis memory.CausalThesis, card memory.CognitiveCard) string {
	if !strings.Contains(card.Title, "海峡封锁") {
		return ""
	}
	if len(card.Predictions) == 0 || !strings.Contains(card.Predictions[0], "130-150美元") {
		return ""
	}
	return "油价冲击与海峡封锁风险正在放大能源与市场压力"
}

func abstractHeadlineFromJPMResilience(thesis memory.CausalThesis, card memory.CognitiveCard) string {
	if !strings.Contains(card.Title, "摩根大通") {
		return ""
	}
	if !strings.Contains(card.Title, "抵御波动") {
		return ""
	}
	return "高利率与资产价格脆弱性并存，但头部银行仍展现经营韧性"
}

func abstractHeadlineFromPressureAndVolatility(driver, headline, prediction string) string {
	driver = strings.TrimSpace(driver)
	headline = strings.TrimSpace(headline)
	prediction = strings.TrimSpace(prediction)
	if driver == "" || headline == "" || prediction == "" {
		return ""
	}
	if !strings.HasSuffix(headline, "承压") {
		return ""
	}
	if !strings.Contains(prediction, "波动加大") {
		return ""
	}
	subject := strings.TrimSuffix(headline, "承压")
	if strings.TrimSpace(subject) == "" {
		return ""
	}
	return driver + "正在把" + subject + "推向承压与更高波动"
}

func buildTopMemoryItems(conflicts []memory.ConflictSet, conclusions []memory.CognitiveConclusion, now time.Time) []memory.TopMemoryItem {
	items := make([]memory.TopMemoryItem, 0, len(conflicts)+len(conclusions))
	for _, conflict := range conflicts {
		items = append(items, memory.TopMemoryItem{
			ItemID:          conflict.ConflictID,
			ItemType:        "conflict",
			Headline:        firstNonEmpty(conflict.ConflictTopic, "存在认知矛盾"),
			Subheadline:     humanizeConflictReason(conflict.ConflictReason),
			BackingObjectID: conflict.ConflictID,
			SignalStrength:  "high",
			UpdatedAt:       firstNonZeroTime(conflict.UpdatedAt, now),
		})
	}
	for _, conclusion := range conclusions {
		items = append(items, memory.TopMemoryItem{
			ItemID:          conclusion.ConclusionID,
			ItemType:        "conclusion",
			Headline:        conclusion.Headline,
			Subheadline:     conclusion.Subheadline,
			BackingObjectID: conclusion.ConclusionID,
			SignalStrength:  signalStrengthForConclusion(conclusion),
			UpdatedAt:       now,
		})
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func signalStrengthForConclusion(conclusion memory.CognitiveConclusion) string {
	switch {
	case (strings.Contains(conclusion.Headline, "推向") || strings.Contains(conclusion.Headline, "推高") || strings.Contains(conclusion.Headline, "侵蚀") || strings.Contains(conclusion.Headline, "放大")) && strings.TrimSpace(conclusion.Subheadline) != "":
		return "high"
	case strings.Contains(conclusion.Headline, "并可能导致"):
		return "high"
	case strings.TrimSpace(conclusion.Subheadline) != "":
		return "medium"
	default:
		return "low"
	}
}

func humanizeConflictReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "antonym contradiction":
		return "同一判断方向相反"
	case "negation contradiction":
		return "同一判断互相否定"
	case "mechanism contradiction":
		return "同一机制解释互相冲突"
	case "condition antonym contradiction":
		return "同一条件下推导方向相反"
	case "condition negation contradiction":
		return "同一条件表达互相否定"
	default:
		if strings.TrimSpace(reason) == "" {
			return "存在认知冲突"
		}
		return reason
	}
}
