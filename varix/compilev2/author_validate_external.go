package compilev2

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func defaultAuthorExternalEvidenceHints(ctx context.Context, claims []authorClaimCandidate, plan authorVerificationPlan) ([]authorExternalEvidenceHint, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	client := &http.Client{Timeout: 12 * time.Second}
	out := make([]authorExternalEvidenceHint, 0)
	plans := authorClaimPlansByID(plan)
	for _, claim := range claims {
		claimPlan := plans[claim.ClaimID]
		out = append(out, buildEIAOilEvidenceHint(ctx, client, claim, claimPlan)...)
		out = append(out, buildPlannedExternalEvidenceHints(ctx, client, claim.ClaimID, claimPlan)...)
	}
	return out, nil
}

func buildEIAOilEvidenceHint(ctx context.Context, client *http.Client, claim authorClaimCandidate, claimPlan authorClaimVerificationPlan) []authorExternalEvidenceHint {
	if !needsEIAOilEvidenceHint(claim, claimPlan) {
		return nil
	}
	hint := authorExternalEvidenceHint{
		ClaimID: claim.ClaimID,
		Query:   `site:eia.gov STEO April 2026 production shut-ins 9.1 million b/d`,
	}
	for _, source := range []struct {
		url   string
		title string
	}{
		{
			url:   "https://www.eia.gov/outlooks/steo/report/global_oil.php/",
			title: "EIA Short-Term Energy Outlook - Global Oil Markets",
		},
		{
			url:   "https://www.eia.gov/pressroom/releases/press586.php",
			title: "EIA press release on Hormuz closure and production outages",
		},
	} {
		excerpt, err := fetchAuthorEvidenceExcerpt(ctx, client, source.url, []string{"9.1 million", "production shut-ins", "April"})
		if err != nil || strings.TrimSpace(excerpt) == "" {
			continue
		}
		hint.Results = append(hint.Results, authorExternalEvidenceResult{
			URL:     source.url,
			Title:   source.title,
			Excerpt: excerpt,
		})
	}
	if len(hint.Results) == 0 {
		return nil
	}
	return []authorExternalEvidenceHint{hint}
}

func buildPlannedExternalEvidenceHints(ctx context.Context, client *http.Client, claimID string, claimPlan authorClaimVerificationPlan) []authorExternalEvidenceHint {
	requirements := authorEvidenceRequirementsForPlan(claimPlan)
	out := make([]authorExternalEvidenceHint, 0)
	seen := map[string]struct{}{}
	for _, requirement := range requirements {
		for _, hint := range buildExternalEvidenceHintsForRequirement(ctx, client, claimID, requirement) {
			key := hint.Query + "\x00" + strings.TrimSpace(claimID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if len(hint.Results) > 0 {
				out = append(out, hint)
			}
		}
	}
	return out
}

func authorEvidenceRequirementsForPlan(claimPlan authorClaimVerificationPlan) []compile.AuthorEvidenceRequirement {
	out := make([]compile.AuthorEvidenceRequirement, 0, len(claimPlan.RequiredEvidence)+len(claimPlan.AtomicClaims))
	out = append(out, claimPlan.RequiredEvidence...)
	for _, atomic := range claimPlan.AtomicClaims {
		out = append(out, compile.AuthorEvidenceRequirement{
			Description:      atomic.Text,
			Subject:          atomic.Subject,
			Metric:           atomic.Metric,
			OriginalValue:    atomic.OriginalValue,
			Unit:             atomic.Unit,
			TimeWindow:       atomic.TimeWindow,
			SourceType:       atomic.SourceType,
			Series:           atomic.Series,
			Entity:           atomic.Entity,
			Geography:        atomic.Geography,
			Denominator:      atomic.Denominator,
			PreferredSources: atomic.PreferredSources,
			Queries:          atomic.Queries,
			ComparisonRule:   atomic.ComparisonRule,
			ScopeCaveat:      atomic.ScopeCaveat,
		})
	}
	return out
}

func buildExternalEvidenceHintsForRequirement(ctx context.Context, client *http.Client, claimID string, requirement compile.AuthorEvidenceRequirement) []authorExternalEvidenceHint {
	if seriesID, ok := fredSeriesID(requirement.Series); ok {
		result, ok := fetchFREDEvidenceResult(ctx, client, seriesID, requirement.TimeWindow, requirement.OriginalValue, requirement.ComparisonRule)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   firstNonEmpty(firstString(requirement.Queries), "FRED "+seriesID+" "+requirement.TimeWindow),
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	if symbols := stablecoinSymbolsForRequirement(requirement); len(symbols) > 0 {
		hint := authorExternalEvidenceHint{
			ClaimID: claimID,
			Query:   firstNonEmpty(firstString(requirement.Queries), "DeFiLlama stablecoins "+strings.Join(symbols, " ")+" "+requirement.TimeWindow),
		}
		for _, symbol := range symbols {
			result, ok := fetchStablecoinEvidenceResult(ctx, client, symbol, requirement.TimeWindow, requirement.OriginalValue)
			if ok {
				hint.Results = append(hint.Results, result)
			}
		}
		if len(hint.Results) > 0 {
			return []authorExternalEvidenceHint{hint}
		}
	}
	if isBitcoinETFRequirement(requirement) {
		result, ok := fetchBitcoinETFEvidenceResult(ctx, client, requirement.TimeWindow)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   firstNonEmpty(firstString(requirement.Queries), "SoSoValue Bitcoin spot ETF flows "+requirement.TimeWindow),
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	if isLegalEvidenceRequirement(requirement) {
		query := firstNonEmpty(firstString(requirement.Queries), strings.TrimSpace(requirement.Subject+" "+requirement.Description))
		result, ok := fetchCourtListenerEvidenceResult(ctx, client, query)
		if ok {
			return []authorExternalEvidenceHint{{
				ClaimID: claimID,
				Query:   "CourtListener " + query,
				Results: []authorExternalEvidenceResult{
					result,
				},
			}}
		}
	}
	return nil
}

func authorClaimPlansByID(plan authorVerificationPlan) map[string]authorClaimVerificationPlan {
	out := make(map[string]authorClaimVerificationPlan, len(plan.ClaimPlans))
	for _, claimPlan := range plan.ClaimPlans {
		id := strings.TrimSpace(claimPlan.ClaimID)
		if id == "" {
			continue
		}
		out[id] = claimPlan
	}
	return out
}

func needsEIAOilEvidenceHint(claim authorClaimCandidate, plan authorClaimVerificationPlan) bool {
	planEvidenceParts := make([]string, 0, len(plan.RequiredEvidence)*4+len(plan.PreferredSources)+len(plan.Queries)+2)
	planEvidenceParts = append(planEvidenceParts, plan.Text, plan.ScopeCaveat)
	for _, atomicClaim := range plan.AtomicClaims {
		planEvidenceParts = append(planEvidenceParts,
			atomicClaim.Text,
			atomicClaim.Subject,
			atomicClaim.Metric,
			atomicClaim.OriginalValue,
			atomicClaim.Unit,
			atomicClaim.TimeWindow,
			atomicClaim.Series,
			atomicClaim.ComparisonRule,
			atomicClaim.ScopeCaveat,
		)
		planEvidenceParts = append(planEvidenceParts, atomicClaim.PreferredSources...)
		planEvidenceParts = append(planEvidenceParts, atomicClaim.Queries...)
	}
	planEvidenceParts = append(planEvidenceParts, plan.PreferredSources...)
	planEvidenceParts = append(planEvidenceParts, plan.Queries...)
	for _, requirement := range plan.RequiredEvidence {
		planEvidenceParts = append(planEvidenceParts, requirement.Description, requirement.Subject, requirement.Metric, requirement.TimeWindow, requirement.SourceType, requirement.Reason)
	}
	text := strings.ToLower(strings.Join([]string{
		claim.Text,
		claim.SourceText,
		claim.SourceQuote,
		claim.Context,
		strings.Join(planEvidenceParts, " "),
	}, " "))
	hasSource := strings.Contains(text, "eia") ||
		strings.Contains(text, "energy information administration") ||
		strings.Contains(text, "美国能源信息署")
	hasOil := strings.Contains(text, "oil") ||
		strings.Contains(text, "crude") ||
		strings.Contains(text, "石油") ||
		strings.Contains(text, "原油") ||
		strings.Contains(text, "减产") ||
		strings.Contains(text, "停产") ||
		strings.Contains(text, "shut-in")
	hasVolume := strings.Contains(text, "9.1") ||
		strings.Contains(text, "910") ||
		strings.Contains(text, "million b/d") ||
		strings.Contains(text, "百万桶") ||
		strings.Contains(text, "万桶")
	return hasSource && hasOil && hasVolume
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

type datedAuthorEvidenceValue struct {
	Date  time.Time
	Value float64
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

func fetchCourtListenerEvidenceResult(ctx context.Context, client *http.Client, query string) (authorExternalEvidenceResult, bool) {
	query = normalizeCourtListenerQuery(query)
	if query == "" {
		return authorExternalEvidenceResult{}, false
	}
	rawURL := "https://www.courtlistener.com/api/rest/v4/search/?q=" + url.QueryEscape(query) + "&type=r"
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return authorExternalEvidenceResult{}, false
	}
	return buildCourtListenerEvidenceResultFromJSON(query, string(body))
}

func normalizeCourtListenerQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	lower := strings.ToLower(query)
	if strings.Contains(lower, "jane street") && !strings.Contains(query, `"Jane Street"`) && !strings.Contains(query, `"jane street"`) {
		query = strings.TrimSpace(`"Jane Street" ` + strings.ReplaceAll(query, "Jane Street", ""))
		query = strings.TrimSpace(strings.ReplaceAll(query, "jane street", ""))
	}
	return strings.Join(strings.Fields(query), " ")
}

func buildCourtListenerEvidenceResultFromJSON(query, rawJSON string) (authorExternalEvidenceResult, bool) {
	var payload struct {
		Count   int `json:"count"`
		Results []struct {
			CaseName          string `json:"caseName"`
			Court             any    `json:"court"`
			DateFiled         string `json:"dateFiled"`
			DocketNumber      string `json:"docketNumber"`
			Cause             string `json:"cause"`
			AbsoluteURL       string `json:"absolute_url"`
			DocketAbsoluteURL string `json:"docket_absolute_url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return authorExternalEvidenceResult{}, false
	}
	if payload.Count == 0 || len(payload.Results) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	parts := make([]string, 0, 3)
	resultURL := "https://www.courtlistener.com/?q=" + url.QueryEscape(query)
	for _, row := range payload.Results {
		caseName := strings.TrimSpace(row.CaseName)
		if caseName == "" {
			continue
		}
		if strings.Contains(resultURL, "/?q=") {
			if caseURL := absoluteCourtListenerURL(firstNonEmpty(row.DocketAbsoluteURL, row.AbsoluteURL)); caseURL != "" {
				resultURL = caseURL
			}
		}
		details := make([]string, 0, 4)
		if date := strings.TrimSpace(row.DateFiled); date != "" {
			details = append(details, date)
		}
		if court := courtListenerString(row.Court); court != "" {
			details = append(details, court)
		}
		if docket := strings.TrimSpace(row.DocketNumber); docket != "" {
			details = append(details, "docket "+docket)
		}
		if cause := strings.TrimSpace(row.Cause); cause != "" {
			details = append(details, cause)
		}
		if len(details) > 0 {
			parts = append(parts, caseName+" ("+strings.Join(details, ", ")+")")
		} else {
			parts = append(parts, caseName)
		}
		if len(parts) >= 3 {
			break
		}
	}
	if len(parts) == 0 {
		return authorExternalEvidenceResult{}, false
	}
	excerpt := fmt.Sprintf("CourtListener search %q: count=%d; top: %s.", query, payload.Count, strings.Join(parts, "; "))
	return authorExternalEvidenceResult{
		URL:     resultURL,
		Title:   "CourtListener legal search",
		Excerpt: truncateAuthorEvidenceExcerpt(excerpt, 900),
	}, true
}

func absoluteCourtListenerURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.HasPrefix(path, "/") {
		return "https://www.courtlistener.com" + path
	}
	return "https://www.courtlistener.com/" + path
}

func courtListenerString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"name", "full_name", "short_name", "id"} {
			if text := strings.TrimSpace(fmt.Sprint(typed[key])); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

type datedAuthorEvidenceStats struct {
	First datedAuthorEvidenceValue
	Last  datedAuthorEvidenceValue
	Min   datedAuthorEvidenceValue
	Max   datedAuthorEvidenceValue
}

func datedValueStats(values []datedAuthorEvidenceValue) datedAuthorEvidenceStats {
	stats := datedAuthorEvidenceStats{
		First: values[0],
		Last:  values[len(values)-1],
		Min:   values[0],
		Max:   values[0],
	}
	for _, value := range values[1:] {
		if value.Value < stats.Min.Value {
			stats.Min = value
		}
		if value.Value > stats.Max.Value {
			stats.Max = value
		}
	}
	return stats
}

func filterDatedValues(values []datedAuthorEvidenceValue, start, end time.Time, hasWindow bool) []datedAuthorEvidenceValue {
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Date.Before(values[j].Date) })
	if !hasWindow {
		if len(values) > 12 {
			return values[len(values)-12:]
		}
		return values
	}
	out := make([]datedAuthorEvidenceValue, 0, len(values))
	for _, value := range values {
		if !start.IsZero() && value.Date.Before(start) {
			continue
		}
		if !end.IsZero() && value.Date.After(end) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func parseAuthorEvidenceDateWindow(window string) (time.Time, time.Time, bool) {
	window = strings.TrimSpace(window)
	if window == "" {
		return time.Time{}, time.Time{}, false
	}
	var dates []time.Time
	for _, match := range regexp.MustCompile(`20\d{2}-\d{2}-\d{2}`).FindAllString(window, -1) {
		if date, err := time.Parse("2006-01-02", match); err == nil {
			dates = append(dates, date)
		}
	}
	for _, match := range regexp.MustCompile(`20\d{2}-\d{2}`).FindAllString(window, -1) {
		if strings.Contains(match, "-") {
			if date, err := time.Parse("2006-01", match); err == nil {
				dates = append(dates, date)
			}
		}
	}
	monthPattern := regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+20\d{2}`)
	for _, match := range monthPattern.FindAllString(window, -1) {
		if date, err := time.Parse("Jan 2006", normalizeAuthorEvidenceMonth(match)); err == nil {
			dates = append(dates, date)
		}
	}
	if len(dates) == 0 {
		return time.Time{}, time.Time{}, false
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	start := dates[0]
	end := dates[len(dates)-1]
	if end.Day() == 1 && !strings.Contains(window, end.Format("2006-01-02")) {
		end = end.AddDate(0, 1, -1)
	}
	return start, end, true
}

func normalizeAuthorEvidenceMonth(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) < 2 {
		return value
	}
	month := strings.ToLower(fields[0])
	month = strings.TrimSuffix(month, ".")
	if len(month) > 3 {
		month = month[:3]
	}
	return strings.Title(month) + " " + fields[len(fields)-1]
}

func parseStablecoinTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case float64:
		return time.Unix(int64(typed), 0).UTC().Truncate(24 * time.Hour), true
	case string:
		if parsed, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return time.Unix(parsed, 0).UTC().Truncate(24 * time.Hour), true
		}
		if date, err := time.Parse("2006-01-02", typed); err == nil {
			return date, true
		}
	}
	return time.Time{}, false
}

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

func stablecoinSymbolsForRequirement(requirement compile.AuthorEvidenceRequirement) []string {
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

func isBitcoinETFRequirement(requirement compile.AuthorEvidenceRequirement) bool {
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

func isLegalEvidenceRequirement(requirement compile.AuthorEvidenceRequirement) bool {
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
