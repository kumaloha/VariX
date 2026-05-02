package compile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func fetchStablecoinEvidenceResult(ctx context.Context, client *http.Client, symbol, window, originalValue string) (authorExternalEvidenceResult, bool) {
	id, ok := stablecoinID(symbol)
	if !ok {
		return authorExternalEvidenceResult{}, false
	}
	rawURL := "https://stablecoins.llama.fi/stablecoin/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildStablecoinEvidenceResultFromJSON(symbol, window, originalValue, string(body))
}

func buildStablecoinEvidenceResultFromJSON(symbol, window, originalValue, rawJSON string) (authorExternalEvidenceResult, bool) {
	var payload struct {
		Symbol       string `json:"symbol"`
		ChainBalance map[string]struct {
			Tokens []struct {
				Date        any                `json:"date"`
				Circulating map[string]float64 `json:"circulating"`
			} `json:"tokens"`
		} `json:"chainBalances"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return authorExternalEvidenceResult{}, false
	}
	if strings.TrimSpace(symbol) == "" {
		symbol = payload.Symbol
	}
	start, end, hasWindow := parseAuthorEvidenceDateWindow(window)
	byDate := map[time.Time]float64{}
	for _, chain := range payload.ChainBalance {
		for _, token := range chain.Tokens {
			date, ok := parseStablecoinTimestamp(token.Date)
			if !ok {
				continue
			}
			value, ok := token.Circulating["peggedUSD"]
			if !ok {
				continue
			}
			byDate[date] += value
		}
	}
	values := make([]datedAuthorEvidenceValue, 0, len(byDate))
	for date, value := range byDate {
		values = append(values, datedAuthorEvidenceValue{Date: date, Value: value})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Date.Before(values[j].Date) })
	filtered := filterDatedValues(values, start, end, hasWindow)
	if len(filtered) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	stats := datedValueStats(filtered)
	delta := stats.Last.Value - stats.First.Value
	excerpt := fmt.Sprintf("DeFiLlama stablecoin %s circulating supply for %s: first %s=%s, last %s=%s, delta=%s. author value %s.",
		strings.ToUpper(strings.TrimSpace(symbol)),
		firstNonEmpty(strings.TrimSpace(window), "available window"),
		stats.First.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.First.Value),
		stats.Last.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Last.Value),
		formatAuthorEvidenceNumber(delta),
		firstNonEmpty(originalValue, "not specified"),
	)
	return authorExternalEvidenceResult{
		URL:     "https://stablecoins.llama.fi/stablecoin/" + firstNonEmpty(mustStablecoinID(symbol), strings.ToUpper(symbol)),
		Title:   "DeFiLlama stablecoin " + strings.ToUpper(strings.TrimSpace(symbol)),
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}
