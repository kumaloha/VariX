package polling

import (
	"context"
	"fmt"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"strings"
)

func (s *Service) Follow(ctx context.Context, target types.FollowTarget) (types.FollowTarget, error) {
	target, err := normalizeFollowTarget(target)
	if err != nil {
		return types.FollowTarget{}, err
	}
	if target.FollowedAt.IsZero() {
		target.FollowedAt = s.now()
	}
	return target, s.store.RegisterFollow(ctx, target)
}

func (s *Service) FollowURL(ctx context.Context, rawURL string) (types.FollowTarget, error) {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return types.FollowTarget{}, err
	}

	switch {
	case parsed.Platform == types.PlatformRSS || parsed.ContentType == types.ContentTypeFeed:
		if !s.dispatcher.SupportsFollow(types.KindRSS, types.PlatformRSS) {
			return types.FollowTarget{}, fmt.Errorf("follow strategy not supported: rss/rss")
		}
		return s.Follow(ctx, types.FollowTarget{
			Kind:       types.KindRSS,
			Platform:   string(types.PlatformRSS),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	case parsed.ContentType == types.ContentTypeProfile:
		if !s.dispatcher.SupportsFollow(types.KindNative, parsed.Platform) {
			return types.FollowTarget{}, fmt.Errorf("follow strategy not supported: native/%s", parsed.Platform)
		}
		return s.Follow(ctx, types.FollowTarget{
			Kind:       types.KindNative,
			Platform:   string(parsed.Platform),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	default:
		return types.FollowTarget{}, fmt.Errorf("url is not followable: %s", rawURL)
	}
}

func (s *Service) FollowSearch(ctx context.Context, platform types.Platform, query string) (types.FollowTarget, error) {
	clean := strings.Join(strings.Fields(query), " ")
	if strings.TrimSpace(string(platform)) == "" {
		return types.FollowTarget{}, fmt.Errorf("search follow requires platform")
	}
	if !s.dispatcher.SupportsFollow(types.KindSearch, platform) {
		return types.FollowTarget{}, fmt.Errorf("follow strategy not supported: search/%s", platform)
	}
	if clean == "" {
		return types.FollowTarget{}, fmt.Errorf("search follow requires query")
	}

	return s.Follow(ctx, types.FollowTarget{
		Kind:          types.KindSearch,
		Platform:      string(platform),
		Locator:       clean,
		Query:         clean,
		HydrationHint: string(platform),
	})
}

func (s *Service) RemoveFollowURL(ctx context.Context, rawURL string) error {
	parsed, err := s.dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return err
	}

	switch {
	case parsed.Platform == types.PlatformRSS || parsed.ContentType == types.ContentTypeFeed:
		if !s.dispatcher.SupportsFollow(types.KindRSS, types.PlatformRSS) {
			return fmt.Errorf("follow strategy not supported: rss/rss")
		}
		return s.removeFollow(ctx, types.FollowTarget{
			Kind:       types.KindRSS,
			Platform:   string(types.PlatformRSS),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	case parsed.ContentType == types.ContentTypeProfile:
		if !s.dispatcher.SupportsFollow(types.KindNative, parsed.Platform) {
			return fmt.Errorf("follow strategy not supported: native/%s", parsed.Platform)
		}
		return s.removeFollow(ctx, types.FollowTarget{
			Kind:       types.KindNative,
			Platform:   string(parsed.Platform),
			PlatformID: parsed.PlatformID,
			Locator:    parsed.CanonicalURL,
			URL:        parsed.CanonicalURL,
		})
	default:
		return fmt.Errorf("url is not followable: %s", rawURL)
	}
}

func (s *Service) RemoveFollowSearch(ctx context.Context, platform types.Platform, query string) error {
	clean := strings.Join(strings.Fields(query), " ")
	if strings.TrimSpace(string(platform)) == "" {
		return fmt.Errorf("search follow requires platform")
	}
	if !s.dispatcher.SupportsFollow(types.KindSearch, platform) {
		return fmt.Errorf("follow strategy not supported: search/%s", platform)
	}
	if clean == "" {
		return fmt.Errorf("search follow requires query")
	}

	return s.removeFollow(ctx, types.FollowTarget{
		Kind:          types.KindSearch,
		Platform:      string(platform),
		Locator:       clean,
		Query:         clean,
		HydrationHint: string(platform),
	})
}

func (s *Service) ListFollows(ctx context.Context) ([]types.FollowTarget, []contentstore.ScanWarning, error) {
	return s.store.ListFollows(ctx)
}

func (s *Service) removeFollow(ctx context.Context, target types.FollowTarget) error {
	target, err := normalizeFollowTarget(target)
	if err != nil {
		return err
	}
	return s.store.RemoveFollow(ctx, target.Kind, target.Platform, target.Locator)
}

func normalizeFollowTarget(target types.FollowTarget) (types.FollowTarget, error) {
	target.Platform = strings.TrimSpace(target.Platform)
	target.PlatformID = strings.TrimSpace(target.PlatformID)
	target.URL = strings.TrimSpace(target.URL)
	target.Query = strings.Join(strings.Fields(target.Query), " ")
	target.Locator = strings.Join(strings.Fields(target.Locator), " ")
	target.HydrationHint = strings.TrimSpace(target.HydrationHint)

	switch target.Kind {
	case types.KindRSS:
		target.Platform = string(types.PlatformRSS)
		if target.Locator == "" {
			target.Locator = target.URL
		}
		if target.URL == "" {
			target.URL = target.Locator
		}
	case types.KindNative:
		if target.Locator == "" {
			target.Locator = target.URL
		}
	case types.KindSearch:
		if target.Query == "" {
			target.Query = target.Locator
		}
		if target.Locator == "" {
			target.Locator = target.Query
		}
		if target.HydrationHint == "" {
			target.HydrationHint = target.Platform
		}
	default:
		return types.FollowTarget{}, fmt.Errorf("unsupported follow kind: %s", target.Kind)
	}

	if strings.TrimSpace(string(target.Kind)) == "" || target.Platform == "" || target.Locator == "" {
		return types.FollowTarget{}, fmt.Errorf("invalid follow target")
	}
	return target, nil
}

func followTargetKey(target types.FollowTarget) string {
	return string(target.Kind) + ":" + target.Platform + ":" + target.Locator
}
