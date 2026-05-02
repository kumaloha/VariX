package verify

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/search"
	websource "github.com/kumaloha/VariX/varix/ingest/sources/web"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

const (
	maxFactRetrievalNodes          = 3
	maxFactRetrievalResultsPerNode = 2
)

func EnableFactWebVerification() {
	buildFactRetrievalContext = defaultFactRetrievalContext
}

func defaultFactRetrievalContext(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	searchCollector := search.NewGoogle(types.PlatformWeb, "", client)
	webCollector := websource.New(client)

	out := make([]map[string]any, 0, len(nodes))
	for i, node := range nodes {
		if i >= maxFactRetrievalNodes {
			break
		}
		query := strings.TrimSpace(node.Text)
		if query == "" {
			continue
		}
		if author := strings.TrimSpace(bundle.AuthorName); author != "" {
			query += " " + author
		}
		discovered, err := searchCollector.Discover(ctx, types.FollowTarget{
			Kind:     types.KindSearch,
			Platform: "web",
			Query:    query,
			Locator:  query,
		})
		if err != nil || len(discovered) == 0 {
			continue
		}

		results := make([]map[string]any, 0, maxFactRetrievalResultsPerNode)
		resultsLimited := len(discovered) > maxFactRetrievalResultsPerNode
		excerptTruncated := false
		for j, item := range discovered {
			if j >= maxFactRetrievalResultsPerNode {
				break
			}
			result := map[string]any{"url": item.URL}
			raws, err := webCollector.Fetch(ctx, types.ParsedURL{
				Platform:     types.PlatformWeb,
				ContentType:  types.ContentTypePost,
				PlatformID:   item.URL,
				CanonicalURL: item.URL,
			})
			if err == nil && len(raws) > 0 {
				raw := raws[0]
				if raw.Metadata.Web != nil && strings.TrimSpace(raw.Metadata.Web.Title) != "" {
					result["title"] = strings.TrimSpace(raw.Metadata.Web.Title)
				}
				excerpt := strings.TrimSpace(raw.ExpandedText())
				if excerpt == "" {
					excerpt = strings.TrimSpace(raw.Content)
				}
				if excerpt != "" {
					truncatedExcerpt, truncated := truncateVerifierExcerpt(excerpt, 600)
					result["excerpt"] = truncatedExcerpt
					if truncated {
						result["excerpt_truncated"] = true
						excerptTruncated = true
					}
				}
			}
			results = append(results, result)
		}
		if len(results) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"node_id":           node.ID,
			"results":           results,
			"results_limited":   resultsLimited,
			"excerpt_truncated": excerptTruncated,
		})
	}
	return out, nil
}

func truncateVerifierExcerpt(text string, max int) (string, bool) {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text, false
	}
	return strings.TrimSpace(text[:max]) + "…", true
}
