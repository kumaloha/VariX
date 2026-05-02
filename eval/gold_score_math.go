package eval

import (
	"math"
)

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
