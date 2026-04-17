package dispatcher

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/sources"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type ItemSource = sources.ItemSource
type Discoverer = sources.Discoverer
type Parser func(raw string) (types.ParsedURL, error)

type Service struct {
	parse       Parser
	resolver    provenance.LinkResolver
	itemSources map[types.Platform]ItemSource
	discoverers map[string]Discoverer
}

func New(parse Parser, itemSources []ItemSource, discoverers []Discoverer, resolver provenance.LinkResolver) *Service {
	svc := &Service{
		parse:       parse,
		resolver:    resolver,
		itemSources: make(map[types.Platform]ItemSource, len(itemSources)),
		discoverers: make(map[string]Discoverer, len(discoverers)),
	}
	for _, src := range itemSources {
		svc.itemSources[src.Platform()] = src
	}
	for _, src := range discoverers {
		svc.discoverers[discovererKey(src.Kind(), src.Platform())] = src
	}
	return svc
}

func (s *Service) SupportsFollow(kind types.Kind, platform types.Platform) bool {
	_, ok := s.discoverers[discovererKey(kind, platform)]
	return ok
}

func (s *Service) ParseURL(ctx context.Context, raw string) (types.ParsedURL, error) {
	if s.parse == nil {
		return types.ParsedURL{}, fmt.Errorf("no parser configured")
	}
	return s.resolveAndParse(ctx, raw)
}

func (s *Service) FetchByParsedURL(ctx context.Context, parsed types.ParsedURL) ([]types.RawContent, error) {
	src, ok := s.itemSources[parsed.Platform]
	if !ok && parsed.Platform == types.PlatformRSS {
		if webSrc, webOK := s.itemSources[types.PlatformWeb]; webOK {
			parsed.Platform = types.PlatformWeb
			parsed.ContentType = types.ContentTypePost
			return webSrc.Fetch(ctx, parsed)
		}
	}
	if !ok {
		return nil, fmt.Errorf("no item source registered for %s", parsed.Platform)
	}
	return src.Fetch(ctx, parsed)
}

func (s *Service) DiscoverFollowedTarget(ctx context.Context, target types.FollowTarget) ([]types.DiscoveryItem, error) {
	src, ok := s.discoverers[discovererKey(target.Kind, types.Platform(target.Platform))]
	if !ok {
		return nil, fmt.Errorf("no discoverer registered for %s/%s", target.Kind, target.Platform)
	}
	return src.Discover(ctx, target)
}

func (s *Service) FetchDiscoveryItem(ctx context.Context, item types.DiscoveryItem) ([]types.RawContent, error) {
	if s.parse == nil {
		return nil, fmt.Errorf("no parser configured")
	}
	parsed, err := s.resolveAndParse(ctx, item.URL)
	if err != nil {
		return nil, err
	}
	originalParsed := parsed
	if targetPlatform, ok := overridePlatformForDiscoveryItem(originalParsed, item); ok {
		parsed.Platform = targetPlatform
	}
	parsed.ContentType = types.ContentTypePost
	decisionMode := ""
	decisionReason := ""
	if item.ExternalID != "" && !hasTrustedNativeItemID(originalParsed, parsed.Platform) && !hasStableWebIdentity(originalParsed, parsed.Platform) {
		parsed.PlatformID = item.ExternalID
		decisionMode = "used_discovery_external_id"
		decisionReason = "fallback_allowed"
	} else if item.ExternalID != "" {
		decisionMode = "retained_parsed_identity"
		switch {
		case hasTrustedNativeItemID(originalParsed, parsed.Platform):
			decisionReason = "trusted_native_id"
		case hasStableWebIdentity(originalParsed, parsed.Platform):
			decisionReason = "stable_web_identity"
		default:
			decisionReason = "parsed_identity_preferred"
		}
	}
	items, err := s.FetchByParsedURL(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if decisionMode == "" {
		return items, nil
	}
	return appendDiscoveryDecisionEvidence(items, originalParsed, parsed, item, decisionMode, decisionReason), nil
}

func overridePlatformForDiscoveryItem(parsed types.ParsedURL, item types.DiscoveryItem) (types.Platform, bool) {
	targetPlatform := types.Platform("")
	switch {
	case item.HydrationHint != "":
		targetPlatform = types.Platform(item.HydrationHint)
	case item.Platform != "" && item.Platform != types.PlatformRSS:
		targetPlatform = item.Platform
	default:
		return "", false
	}

	// Feed discovery items are expected to hydrate into individual posts, not feed XML.
	if parsed.Platform == types.PlatformRSS {
		return targetPlatform, true
	}
	// Generic web search results should not downgrade URLs the parser already
	// recognized as native platform items.
	if targetPlatform == types.PlatformWeb && parsed.Platform != types.PlatformWeb {
		return "", false
	}
	if targetPlatform == parsed.Platform {
		return targetPlatform, true
	}
	if hasTrustedNativeItemID(parsed, targetPlatform) {
		return targetPlatform, true
	}
	// Generic web URLs should only be forced into a native collector when the
	// discovery layer already carries a concrete native identity.
	if parsed.Platform == types.PlatformWeb && strings.TrimSpace(item.ExternalID) == "" {
		return "", false
	}
	return targetPlatform, true
}

func (s *Service) resolveAndParse(ctx context.Context, raw string) (types.ParsedURL, error) {
	if s.parse == nil {
		return types.ParsedURL{}, fmt.Errorf("no parser configured")
	}
	if s.resolver != nil && shouldResolveBeforeParse(raw) {
		resolved, err := s.resolver.Resolve(ctx, raw)
		if err == nil && strings.TrimSpace(resolved) != "" {
			raw = resolved
		}
	}
	return s.parse(raw)
}

func discovererKey(kind types.Kind, platform types.Platform) string {
	return string(kind) + ":" + string(platform)
}

func shouldResolveBeforeParse(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "mapp.api.weibo.cn", "t.co", "bit.ly", "weibo.cn":
		return true
	default:
		return false
	}
}

func hasTrustedNativeItemID(parsed types.ParsedURL, targetPlatform types.Platform) bool {
	if parsed.PlatformID == "" {
		return false
	}
	if parsed.Platform != targetPlatform {
		return false
	}
	switch targetPlatform {
	case types.PlatformTwitter, types.PlatformWeibo, types.PlatformYouTube, types.PlatformBilibili:
		return true
	default:
		return false
	}
}

// hasStableWebIdentity returns true when the URL parser already produced a
// deterministic web identity (md5 of URL) and the resolved platform is still
// web.  In that case we must NOT overwrite PlatformID with the discovery-layer
// ExternalID (e.g. an RSS guid), because the polling dedupe check uses the
// parser-derived identity.
func hasStableWebIdentity(parsed types.ParsedURL, resolvedPlatform types.Platform) bool {
	return parsed.Platform == types.PlatformWeb && parsed.PlatformID != "" && resolvedPlatform == types.PlatformWeb
}

func appendDiscoveryDecisionEvidence(items []types.RawContent, originalParsed, resolvedParsed types.ParsedURL, item types.DiscoveryItem, mode, reason string) []types.RawContent {
	if len(items) == 0 {
		return items
	}
	out := make([]types.RawContent, 0, len(items))
	value := fmt.Sprintf(
		"mode=%s reason=%s parsed_platform=%s parsed_id=%s resolved_platform=%s resolved_id=%s external_id=%s url=%s",
		mode,
		reason,
		originalParsed.Platform,
		originalParsed.PlatformID,
		resolvedParsed.Platform,
		resolvedParsed.PlatformID,
		item.ExternalID,
		item.URL,
	)
	for _, raw := range items {
		raw.Provenance = provenance.AppendEvidence(raw.Provenance, types.ProvenanceEvidence{
			Kind:   "discovery_identity_decision",
			Value:  value,
			Weight: string(types.ConfidenceHigh),
		})
		out = append(out, raw)
	}
	return out
}
