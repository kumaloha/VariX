package compilev2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func fetchBitcoinETFEvidenceResult(ctx context.Context, client *http.Client, window string) (authorExternalEvidenceResult, bool) {
	body := bytes.NewBufferString(`{}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart", body)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildBitcoinETFEvidenceResultFromSoSoValueJSON(window, string(raw))
}

func buildBitcoinETFEvidenceResultFromSoSoValueJSON(window, rawJSON string) (authorExternalEvidenceResult, bool) {
	var payload struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return authorExternalEvidenceResult{}, false
	}
	var rows []struct {
		Date           string  `json:"date"`
		TotalNetInflow float64 `json:"totalNetInflow"`
	}
	if len(payload.Data) > 0 && payload.Data[0] == '{' {
		var wrapped struct {
			List []struct {
				Date           string  `json:"date"`
				TotalNetInflow float64 `json:"totalNetInflow"`
			} `json:"list"`
		}
		if err := json.Unmarshal(payload.Data, &wrapped); err == nil {
			rows = wrapped.List
		}
	} else if len(payload.Data) > 0 {
		_ = json.Unmarshal(payload.Data, &rows)
	}
	if len(rows) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	start, end, hasWindow := parseAuthorEvidenceDateWindow(window)
	values := make([]datedAuthorEvidenceValue, 0, len(rows))
	for _, row := range rows {
		date, err := time.Parse("2006-01-02", strings.TrimSpace(row.Date))
		if err != nil {
			continue
		}
		values = append(values, datedAuthorEvidenceValue{Date: date, Value: row.TotalNetInflow})
	}
	filtered := filterDatedValues(values, start, end, hasWindow)
	if len(filtered) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	stats := datedValueStats(filtered)
	var sum float64
	var positiveDays, negativeDays, zeroDays int
	for _, value := range filtered {
		sum += value.Value
		switch {
		case value.Value > 0:
			positiveDays++
		case value.Value < 0:
			negativeDays++
		default:
			zeroDays++
		}
	}
	continuousOutflow := positiveDays == 0 && negativeDays > 0
	excerpt := fmt.Sprintf("SoSoValue BTC spot ETF flows for %s: first %s=%s, last %s=%s, sum=%s, positive_days=%d, negative_days=%d, zero_days=%d, min %s=%s, max %s=%s, continuous_outflow=%t.",
		firstNonEmpty(strings.TrimSpace(window), "available window"),
		stats.First.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.First.Value),
		stats.Last.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Last.Value),
		formatAuthorEvidenceNumber(sum),
		positiveDays,
		negativeDays,
		zeroDays,
		stats.Min.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Min.Value),
		stats.Max.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Max.Value),
		continuousOutflow,
	)
	return authorExternalEvidenceResult{
		URL:     "https://open.sosovalue.xyz/openapi/v1/etf/us-btc-spot/historicalInflowChart",
		Title:   "SoSoValue BTC spot ETF flows",
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}
