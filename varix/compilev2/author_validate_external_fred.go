package compilev2

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func fetchFREDEvidenceResult(ctx context.Context, client *http.Client, seriesID, window, originalValue, comparisonRule string) (authorExternalEvidenceResult, bool) {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return authorExternalEvidenceResult{}, false
	}
	rawURL := "https://fred.stlouisfed.org/graph/fredgraph.csv?id=" + seriesID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authorExternalEvidenceResult{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildFREDEvidenceResultFromCSV(seriesID, window, originalValue, comparisonRule, string(body))
}

func buildFREDEvidenceResultFromCSV(seriesID, window, originalValue, comparisonRule, rawCSV string) (authorExternalEvidenceResult, bool) {
	observations, ok := parseFREDCSV(seriesID, rawCSV)
	if !ok {
		return authorExternalEvidenceResult{}, false
	}
	start, end, hasWindow := parseAuthorEvidenceDateWindow(window)
	filtered := filterDatedValues(observations, start, end, hasWindow)
	if len(filtered) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	stats := datedValueStats(filtered)
	excerpt := fmt.Sprintf("FRED %s observations for %s: first %s=%s, last %s=%s, min %s=%s, max %s=%s. author value %s. Comparison rule: %s.",
		seriesID,
		firstNonEmpty(strings.TrimSpace(window), "available window"),
		stats.First.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.First.Value),
		stats.Last.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Last.Value),
		stats.Min.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Min.Value),
		stats.Max.Date.Format("2006-01-02"), formatAuthorEvidenceNumber(stats.Max.Value),
		firstNonEmpty(originalValue, "not specified"),
		firstNonEmpty(comparisonRule, "compare source values to author value and time window"),
	)
	return authorExternalEvidenceResult{
		URL:     "https://fred.stlouisfed.org/series/" + seriesID,
		Title:   "FRED " + seriesID,
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}

func parseFREDCSV(seriesID, rawCSV string) ([]datedAuthorEvidenceValue, bool) {
	reader := csv.NewReader(bytes.NewBufferString(rawCSV))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, false
	}
	valueColumn := 1
	if len(records[0]) > 1 {
		for i, header := range records[0] {
			if strings.EqualFold(strings.TrimSpace(header), strings.TrimSpace(seriesID)) {
				valueColumn = i
				break
			}
		}
	}
	out := make([]datedAuthorEvidenceValue, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) <= valueColumn {
			continue
		}
		date, err := time.Parse("2006-01-02", strings.TrimSpace(record[0]))
		if err != nil {
			continue
		}
		valueText := strings.TrimSpace(record[valueColumn])
		if valueText == "" || valueText == "." {
			continue
		}
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil {
			continue
		}
		out = append(out, datedAuthorEvidenceValue{Date: date, Value: value})
	}
	return out, len(out) > 0
}
