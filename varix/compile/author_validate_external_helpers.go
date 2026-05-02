package compile

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

func fredSeriesID(series string) (string, bool) {
	series = strings.TrimSpace(series)
	if series == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToUpper(series), "FRED:") {
		return strings.TrimSpace(series[5:]), true
	}
	return "", false
}

func stablecoinSymbolsForRequirement(requirement AuthorEvidenceRequirement) []string {
	text := strings.ToUpper(strings.Join([]string{requirement.Subject, requirement.Metric, requirement.Description, requirement.Series}, " "))
	out := make([]string, 0, 2)
	if strings.Contains(text, "USDT") || strings.Contains(text, "STABLECOIN") || strings.Contains(text, "稳定币") {
		out = append(out, "USDT")
	}
	if strings.Contains(text, "USDC") || strings.Contains(text, "STABLECOIN") || strings.Contains(text, "稳定币") {
		out = append(out, "USDC")
	}
	return out
}

func isBitcoinETFRequirement(requirement AuthorEvidenceRequirement) bool {
	text := strings.ToLower(strings.Join([]string{
		requirement.Subject,
		requirement.Metric,
		requirement.Description,
		requirement.Series,
		strings.Join(requirement.PreferredSources, " "),
	}, " "))
	hasETF := strings.Contains(text, "etf")
	hasBitcoin := strings.Contains(text, "bitcoin") || strings.Contains(text, "btc") || strings.Contains(text, "比特币")
	hasFlow := strings.Contains(text, "flow") || strings.Contains(text, "inflow") || strings.Contains(text, "outflow") || strings.Contains(text, "流入") || strings.Contains(text, "流出")
	return hasETF && hasBitcoin && hasFlow
}

func isLegalEvidenceRequirement(requirement AuthorEvidenceRequirement) bool {
	text := strings.ToLower(strings.Join([]string{
		requirement.Subject,
		requirement.Metric,
		requirement.Description,
		requirement.Series,
		requirement.SourceType,
		requirement.Entity,
		requirement.Geography,
		requirement.ComparisonRule,
		requirement.ScopeCaveat,
		strings.Join(requirement.PreferredSources, " "),
		strings.Join(requirement.Queries, " "),
	}, " "))
	return containsAny(text,
		"courtlistener",
		"pacer",
		"legal",
		"lawsuit",
		"class action",
		"filing",
		"complaint",
		"allegation",
		"jane street",
		"securities exchange act",
		"诉讼",
		"起诉",
		"集体诉讼",
		"操纵",
	)
}

func stablecoinID(symbol string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(symbol)) {
	case "USDT", "TETHER":
		return "1", true
	case "USDC", "USD COIN":
		return "2", true
	default:
		return "", false
	}
}

func mustStablecoinID(symbol string) string {
	id, _ := stablecoinID(symbol)
	return id
}

func formatAuthorEvidenceNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

var (
	authorEvidenceScriptPattern = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	authorEvidenceTagPattern    = regexp.MustCompile(`(?s)<[^>]+>`)
	authorEvidenceSpacePattern  = regexp.MustCompile(`\s+`)
)

func stripAuthorEvidenceHTML(raw string) string {
	raw = authorEvidenceScriptPattern.ReplaceAllString(raw, " ")
	raw = authorEvidenceTagPattern.ReplaceAllString(raw, " ")
	replacements := []struct {
		old string
		new string
	}{
		{"&nbsp;", " "},
		{"&amp;", "&"},
		{"&quot;", `"`},
		{"&#39;", "'"},
		{"&lt;", "<"},
		{"&gt;", ">"},
	}
	for _, replacement := range replacements {
		raw = strings.ReplaceAll(raw, replacement.old, replacement.new)
	}
	return raw
}

func compactAuthorEvidenceText(text string) string {
	return strings.TrimSpace(authorEvidenceSpacePattern.ReplaceAllString(text, " "))
}

func truncateAuthorEvidenceExcerpt(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func fetchAuthorEvidenceExcerpt(ctx context.Context, client *http.Client, rawURL string, keywords []string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch evidence hint: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	text := compactAuthorEvidenceText(stripAuthorEvidenceHTML(string(body)))
	if text == "" {
		return "", nil
	}
	lower := strings.ToLower(text)
	idx := -1
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" {
			continue
		}
		if found := strings.Index(lower, keyword); found >= 0 {
			idx = found
			break
		}
	}
	if idx < 0 {
		return truncateAuthorEvidenceExcerpt(text, 900), nil
	}
	start := idx - 350
	if start < 0 {
		start = 0
	}
	end := idx + 650
	if end > len(text) {
		end = len(text)
	}
	return truncateAuthorEvidenceExcerpt(strings.TrimSpace(text[start:end]), 900), nil
}
