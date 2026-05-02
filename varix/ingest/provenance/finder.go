package provenance

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type LinkResolver interface {
	Resolve(ctx context.Context, raw string) (string, error)
}

type RuleFinder struct {
	resolver LinkResolver
}

func NewRuleFinder() RuleFinder {
	return NewRuleFinderWithResolver(NewHTTPResolver(httputil.NewPublicHTTPClient(15*time.Second, nil)))
}

func NewRuleFinderWithResolver(resolver LinkResolver) RuleFinder {
	return RuleFinder{resolver: resolver}
}

func (f RuleFinder) FindCandidates(ctx context.Context, raw types.RawContent) ([]types.SourceCandidate, error) {
	candidates := append([]types.SourceCandidate(nil), rawSourceCandidates(raw)...)
	candidates = mergeCandidates(candidates, f.linkCandidates(ctx, raw))
	return candidates, nil
}

func rawSourceCandidates(raw types.RawContent) []types.SourceCandidate {
	if raw.Provenance == nil || len(raw.Provenance.SourceCandidates) == 0 {
		return nil
	}
	out := make([]types.SourceCandidate, 0, len(raw.Provenance.SourceCandidates))
	rawHost := hostFromURL(raw.URL)
	for _, candidate := range raw.Provenance.SourceCandidates {
		if shouldFilterSelfLink(rawHost, candidate.URL) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func (f RuleFinder) linkCandidates(ctx context.Context, raw types.RawContent) []types.SourceCandidate {
	seen := map[string]struct{}{}
	out := make([]types.SourceCandidate, 0)
	rawHost := hostFromURL(raw.URL)
	for _, candidate := range structuredSourceCandidates(raw) {
		link := strings.TrimSpace(candidate.URL)
		if link == "" {
			continue
		}
		link = resolveCandidateLink(ctx, f.resolver, link)
		if shouldFilterSelfLink(rawHost, link) {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		candidate.URL = link
		candidate.Host = hostFromURL(link)
		out = append(out, candidate)
	}
	return out
}

type HTTPResolver struct {
	client *http.Client
}

func NewHTTPResolver(client *http.Client) HTTPResolver {
	return HTTPResolver{client: client}
}

func (r HTTPResolver) Resolve(ctx context.Context, raw string) (string, error) {
	client := r.client
	if client == nil {
		client = httputil.NewPublicHTTPClient(15*time.Second, nil)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := doResolveRequest(resolveCtx, client, http.MethodHead, raw)
	if err != nil {
		return resolveWithMethod(resolveCtx, client, http.MethodGet, raw)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		resp.Body.Close()
		return resolveWithMethod(resolveCtx, client, http.MethodGet, raw)
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String(), nil
	}
	return raw, nil
}

func resolveWithMethod(ctx context.Context, client *http.Client, method, raw string) (string, error) {
	resp, err := doResolveRequest(ctx, client, method, raw)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String(), nil
	}
	return raw, nil
}

func doResolveRequest(ctx context.Context, client *http.Client, method, raw string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, raw, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func resolveCandidateLink(ctx context.Context, resolver LinkResolver, raw string) string {
	if resolver == nil {
		return raw
	}
	resolved, err := resolver.Resolve(ctx, raw)
	if err != nil || strings.TrimSpace(resolved) == "" {
		return raw
	}
	return normalizeResolvedCandidateLink(raw, resolved)
}

func normalizeResolvedCandidateLink(raw string, resolved string) string {
	parsed, err := url.Parse(resolved)
	if err != nil {
		return resolved
	}
	if strings.EqualFold(parsed.Hostname(), "passport.weibo.com") && strings.HasPrefix(parsed.Path, "/visitor/visitor") {
		if wrapped := strings.TrimSpace(parsed.Query().Get("url")); wrapped != "" {
			return wrapped
		}
		return raw
	}
	return resolved
}

func shouldFilterSelfLink(rawHost, link string) bool {
	linkHost := hostFromURL(link)
	if rawHost == "" || linkHost == "" || rawHost != linkHost {
		return false
	}
	lower := strings.ToLower(link)
	return strings.Contains(lower, "/channel/") ||
		strings.Contains(lower, "/playlist") ||
		strings.Contains(lower, "/join") ||
		strings.Contains(lower, "list=")
}

func sourceLinks(raw types.RawContent) []string {
	links := make([]string, 0)
	switch {
	case raw.Metadata.YouTube != nil:
		links = append(links, raw.Metadata.YouTube.SourceLinks...)
	case raw.Metadata.Bilibili != nil:
		links = append(links, raw.Metadata.Bilibili.SourceLinks...)
	case raw.Metadata.Twitter != nil:
		links = append(links, raw.Metadata.Twitter.SourceLinks...)
	case raw.Metadata.Weibo != nil:
		if raw.Metadata.Weibo.OriginalURL != "" {
			links = append(links, raw.Metadata.Weibo.OriginalURL)
		}
	case raw.Metadata.Web != nil:
		if raw.Metadata.Web.YouTubeRedirect != "" {
			links = append(links, raw.Metadata.Web.YouTubeRedirect)
		}
	}
	return links
}
