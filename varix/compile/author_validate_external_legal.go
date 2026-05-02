package compile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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
