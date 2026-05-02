package polling

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func (s *Service) FollowAuthor(ctx context.Context, req types.AuthorFollowRequest) (types.AuthorFollowResult, error) {
	req = normalizeAuthorFollowRequest(req)
	if req.ProfileURL != "" {
		parsed, err := s.dispatcher.ParseURL(ctx, req.ProfileURL)
		if err != nil {
			return types.AuthorFollowResult{}, err
		}
		derivedProfile := false
		if parsedReq, ok := authorFollowRequestFromParsedURL(parsed); ok {
			if req.Platform == "" {
				req.Platform = parsedReq.Platform
			}
			if req.PlatformID == "" {
				req.PlatformID = parsedReq.PlatformID
			}
			if req.ProfileURL == "" || parsedReq.ProfileURL != "" {
				req.ProfileURL = parsedReq.ProfileURL
				derivedProfile = true
			}
		}
		if req.Platform == "" {
			req.Platform = parsed.Platform
		}
		if req.PlatformID == "" {
			req.PlatformID = parsed.PlatformID
		}
		if parsed.CanonicalURL != "" && !derivedProfile {
			req.ProfileURL = parsed.CanonicalURL
		}
		if parsed.Platform == types.PlatformRSS || parsed.ContentType == types.ContentTypeFeed {
			req.Platform = types.PlatformRSS
			req.PlatformID = parsed.PlatformID
			req = normalizeAuthorFollowRequest(req)
			return s.followAuthorRSS(ctx, req, parsed.CanonicalURL)
		}
	}
	req = normalizeAuthorFollowRequest(req)
	if req.Platform == "" {
		return types.AuthorFollowResult{}, fmt.Errorf("author follow requires platform or profile url")
	}

	if rssURL, ok := authorRSSURL(req); ok {
		return s.followAuthorRSS(ctx, req, rssURL)
	}
	if shouldFollowAuthorNative(req) {
		return s.followAuthorNative(ctx, req)
	}
	return s.followAuthorSearch(ctx, req)
}

func (s *Service) ListAuthorSubscriptions(ctx context.Context) ([]types.AuthorSubscription, []contentstore.ScanWarning, error) {
	return s.store.ListAuthorSubscriptions(ctx)
}

func (s *Service) followAuthorRSS(ctx context.Context, req types.AuthorFollowRequest, rssURL string) (types.AuthorFollowResult, error) {
	if !s.dispatcher.SupportsFollow(types.KindRSS, types.PlatformRSS) {
		return types.AuthorFollowResult{}, fmt.Errorf("follow strategy not supported: rss/rss")
	}
	sub, err := s.store.RegisterAuthorSubscription(ctx, types.AuthorSubscription{
		Platform:   req.Platform,
		AuthorName: req.AuthorName,
		PlatformID: req.PlatformID,
		ProfileURL: req.ProfileURL,
		Strategy:   types.SubscriptionStrategyRSS,
		RSSURL:     rssURL,
		Status:     "active",
		UpdatedAt:  s.now(),
	}, nil)
	if err != nil {
		return types.AuthorFollowResult{}, err
	}
	target, err := s.Follow(ctx, types.FollowTarget{
		Kind:       types.KindRSS,
		Platform:   string(types.PlatformRSS),
		PlatformID: req.PlatformID,
		Locator:    rssURL,
		URL:        rssURL,
		AuthorName: req.AuthorName,
	})
	if err != nil {
		return types.AuthorFollowResult{}, err
	}
	return types.AuthorFollowResult{
		Subscription: sub,
		Follows:      []types.FollowTarget{target},
	}, nil
}

func (s *Service) followAuthorNative(ctx context.Context, req types.AuthorFollowRequest) (types.AuthorFollowResult, error) {
	if !s.dispatcher.SupportsFollow(types.KindNative, req.Platform) {
		return types.AuthorFollowResult{}, fmt.Errorf("follow strategy not supported: native/%s", req.Platform)
	}
	profileURL := canonicalAuthorProfileURL(req.Platform, req.PlatformID, req.ProfileURL)
	sub, err := s.store.RegisterAuthorSubscription(ctx, types.AuthorSubscription{
		Platform:   req.Platform,
		AuthorName: req.AuthorName,
		PlatformID: req.PlatformID,
		ProfileURL: profileURL,
		Strategy:   types.SubscriptionStrategyNative,
		Status:     "active",
		UpdatedAt:  s.now(),
	}, nil)
	if err != nil {
		return types.AuthorFollowResult{}, err
	}
	target, err := s.Follow(ctx, types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   string(req.Platform),
		PlatformID: req.PlatformID,
		Locator:    profileURL,
		URL:        profileURL,
		AuthorName: req.AuthorName,
	})
	if err != nil {
		return types.AuthorFollowResult{}, err
	}
	return types.AuthorFollowResult{
		Subscription: sub,
		Follows:      []types.FollowTarget{target},
	}, nil
}

func (s *Service) followAuthorSearch(ctx context.Context, req types.AuthorFollowRequest) (types.AuthorFollowResult, error) {
	if !s.dispatcher.SupportsFollow(types.KindSearch, req.Platform) {
		return types.AuthorFollowResult{}, fmt.Errorf("follow strategy not supported: search/%s", req.Platform)
	}
	queries := AuthorSearchQueries(req)
	if len(queries) == 0 {
		return types.AuthorFollowResult{}, fmt.Errorf("author follow could not derive search queries")
	}
	sub, err := s.store.RegisterAuthorSubscription(ctx, types.AuthorSubscription{
		Platform:   req.Platform,
		AuthorName: req.AuthorName,
		PlatformID: req.PlatformID,
		ProfileURL: req.ProfileURL,
		Strategy:   types.SubscriptionStrategySearch,
		Status:     "active",
		UpdatedAt:  s.now(),
	}, queries)
	if err != nil {
		return types.AuthorFollowResult{}, err
	}

	follows := make([]types.FollowTarget, 0, len(queries))
	for _, query := range queries {
		target, err := s.Follow(ctx, types.FollowTarget{
			Kind:          types.KindSearch,
			Platform:      string(req.Platform),
			Locator:       query.Query,
			Query:         query.Query,
			HydrationHint: string(req.Platform),
			AuthorName:    req.AuthorName,
		})
		if err != nil {
			return types.AuthorFollowResult{}, err
		}
		follows = append(follows, target)
	}
	return types.AuthorFollowResult{
		Subscription: sub,
		Follows:      follows,
		Queries:      queries,
	}, nil
}

func normalizeAuthorFollowRequest(req types.AuthorFollowRequest) types.AuthorFollowRequest {
	req.Platform = types.Platform(strings.TrimSpace(string(req.Platform)))
	req.AuthorName = strings.Join(strings.Fields(req.AuthorName), " ")
	req.PlatformID = canonicalFollowAuthorPlatformID(req.Platform, req.PlatformID)
	req.ProfileURL = strings.TrimSpace(req.ProfileURL)
	return req
}

func canonicalFollowAuthorPlatformID(platform types.Platform, id string) string {
	id = strings.Trim(strings.TrimSpace(id), "@")
	if platform == types.PlatformTwitter {
		return strings.ToLower(id)
	}
	return id
}

func authorRSSURL(req types.AuthorFollowRequest) (string, bool) {
	if req.Platform == types.PlatformYouTube && strings.HasPrefix(req.PlatformID, "UC") {
		return "https://www.youtube.com/feeds/videos.xml?channel_id=" + url.QueryEscape(req.PlatformID), true
	}
	return "", false
}

func shouldFollowAuthorNative(req types.AuthorFollowRequest) bool {
	return req.Platform == types.PlatformWeibo && strings.TrimSpace(req.PlatformID) != ""
}

func AuthorSearchQueries(req types.AuthorFollowRequest) []types.SubscriptionQuery {
	req = normalizeAuthorFollowRequest(req)
	terms := authorSearchTerms(req)
	out := make([]types.SubscriptionQuery, 0, len(terms))
	seen := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		query := strings.Join(strings.Fields(term), " ")
		if query == "" {
			continue
		}
		if _, ok := seen[query]; ok {
			continue
		}
		seen[query] = struct{}{}
		out = append(out, types.SubscriptionQuery{
			Provider: "google",
			Query:    query,
			Priority: len(out) + 1,
		})
	}
	return out
}

func authorSearchTerms(req types.AuthorFollowRequest) []string {
	quotedName := quoteSearchTerm(authorSearchLabel(req))
	id := req.PlatformID
	switch req.Platform {
	case types.PlatformTwitter:
		if id != "" {
			return []string{
				"site:x.com/" + id + "/status",
				"site:twitter.com/" + id + "/status",
			}
		}
		return []string{"site:x.com " + quotedName, "site:twitter.com " + quotedName}
	case types.PlatformWeibo:
		terms := make([]string, 0, 3)
		if id != "" {
			terms = append(terms, "site:weibo.com/"+id)
		}
		if quotedName != "" {
			terms = append(terms, "site:weibo.com "+quotedName, "site:m.weibo.cn/status "+quotedName)
		}
		return terms
	case types.PlatformBilibili:
		terms := make([]string, 0, 2)
		if quotedName != "" {
			terms = append(terms, "site:bilibili.com/video "+quotedName)
		}
		if id != "" {
			terms = append(terms, "site:bilibili.com/video "+quoteSearchTerm("space.bilibili.com/"+id))
		}
		return terms
	case types.PlatformYouTube:
		if quotedName != "" {
			return []string{"site:youtube.com/watch " + quotedName, "site:youtube.com/shorts " + quotedName}
		}
	case types.PlatformWeb:
		if req.ProfileURL != "" {
			if host := profileHost(req.ProfileURL); host != "" {
				return []string{"site:" + host + " " + quotedName}
			}
		}
	}
	if quotedName != "" {
		return []string{quotedName}
	}
	return nil
}

func authorSearchLabel(req types.AuthorFollowRequest) string {
	if strings.TrimSpace(req.AuthorName) != "" {
		return req.AuthorName
	}
	return strings.TrimPrefix(strings.TrimSpace(req.PlatformID), "@")
}

func quoteSearchTerm(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(term, `"`, "") + `"`
}

func profileHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
