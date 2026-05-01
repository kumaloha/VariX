package router

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/kumaloha/VariX/varix/ingest/internal/textutil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

var (
	twitterPost             = regexp.MustCompile(`(?:twitter\.com|x\.com)/(@?[\w]+)/status/(\d+)`)
	twitterProfile          = regexp.MustCompile(`(?:twitter\.com|x\.com)/(@?[\w]+)/?$`)
	weiboPost               = regexp.MustCompile(`weibo\.com/\d+/(\w+)|m\.weibo\.cn/(?:status|detail)/(\w+)`)
	weiboProfile            = regexp.MustCompile(`weibo\.com/(?:u/)?(\d+)/?$`)
	youtubeVideo            = regexp.MustCompile(`(?:youtube\.com/watch\?(?:.*&)?v=|youtu\.be/|youtube\.com/shorts/)([A-Za-z0-9_-]{11})`)
	youtubeChannelProfile   = regexp.MustCompile(`youtube\.com/channel/(UC[A-Za-z0-9_-]+)`)
	youtubeHandleProfile    = regexp.MustCompile(`youtube\.com/@([A-Za-z0-9_.-]+)`)
	bilibiliVideo           = regexp.MustCompile(`bilibili\.com/video/(BV[\w]+)`)
	bilibiliProfile         = regexp.MustCompile(`space\.bilibili\.com/(\d+)`)
	twitterReservedProfiles = map[string]struct{}{
		"about":         {},
		"compose":       {},
		"explore":       {},
		"home":          {},
		"i":             {},
		"login":         {},
		"messages":      {},
		"notifications": {},
		"privacy":       {},
		"search":        {},
		"settings":      {},
		"signup":        {},
		"tos":           {},
	}
	feedSegments = []string{"feed", "rss", "atom"}
)

func Parse(raw string) (types.ParsedURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return types.ParsedURL{}, fmt.Errorf("empty url")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return types.ParsedURL{}, err
	}
	host := strings.ToLower(parsed.Hostname())
	lowerHostPath := strings.ToLower(parsed.Host + parsed.EscapedPath())
	normalizedURL := normalizeURLHost(raw, host)

	if hostMatches(host, "twitter.com", "x.com") {
		if m := twitterPost.FindStringSubmatch(normalizedURL); len(m) > 2 {
			return types.ParsedURL{
				Platform:     types.PlatformTwitter,
				ContentType:  types.ContentTypePost,
				PlatformID:   m[2],
				AuthorID:     strings.TrimPrefix(m[1], "@"),
				CanonicalURL: raw,
			}, nil
		}
		if m := twitterProfile.FindStringSubmatch(lowerHostPath); len(m) > 1 {
			username := strings.TrimPrefix(m[1], "@")
			if isReservedTwitterProfile(username) {
				goto rssOrWeb
			}
			return types.ParsedURL{
				Platform:     types.PlatformTwitter,
				ContentType:  types.ContentTypeProfile,
				PlatformID:   username,
				CanonicalURL: "https://twitter.com/" + username,
			}, nil
		}
	}

	if hostMatches(host, "weibo.com", "m.weibo.cn") {
		if m := weiboPost.FindStringSubmatch(normalizedURL); len(m) > 2 {
			id := textutil.FirstNonEmpty(m[1], m[2])
			return types.ParsedURL{Platform: types.PlatformWeibo, ContentType: types.ContentTypePost, PlatformID: id, CanonicalURL: raw}, nil
		}
		if m := weiboProfile.FindStringSubmatch(normalizedURL); len(m) > 1 {
			id := textutil.FirstNonEmpty(m[1])
			return types.ParsedURL{Platform: types.PlatformWeibo, ContentType: types.ContentTypeProfile, PlatformID: id, CanonicalURL: "https://weibo.com/" + id}, nil
		}
	}

	if hostMatches(host, "youtube.com", "youtu.be") {
		if m := youtubeVideo.FindStringSubmatch(normalizedURL); len(m) > 1 {
			return types.ParsedURL{Platform: types.PlatformYouTube, ContentType: types.ContentTypePost, PlatformID: m[1], CanonicalURL: "https://www.youtube.com/watch?v=" + m[1]}, nil
		}
		if m := youtubeChannelProfile.FindStringSubmatch(normalizedURL); len(m) > 1 {
			return types.ParsedURL{Platform: types.PlatformYouTube, ContentType: types.ContentTypeProfile, PlatformID: m[1], CanonicalURL: "https://www.youtube.com/channel/" + m[1]}, nil
		}
		if m := youtubeHandleProfile.FindStringSubmatch(normalizedURL); len(m) > 1 {
			return types.ParsedURL{Platform: types.PlatformYouTube, ContentType: types.ContentTypeProfile, PlatformID: m[1], CanonicalURL: "https://www.youtube.com/@" + m[1]}, nil
		}
	}

	if hostMatches(host, "bilibili.com") {
		if m := bilibiliVideo.FindStringSubmatch(normalizedURL); len(m) > 1 {
			return types.ParsedURL{Platform: types.PlatformBilibili, ContentType: types.ContentTypePost, PlatformID: m[1], CanonicalURL: "https://www.bilibili.com/video/" + m[1]}, nil
		}
	}
	if hostMatches(host, "space.bilibili.com") {
		if m := bilibiliProfile.FindStringSubmatch(normalizedURL); len(m) > 1 {
			return types.ParsedURL{Platform: types.PlatformBilibili, ContentType: types.ContentTypeProfile, PlatformID: m[1], CanonicalURL: "https://space.bilibili.com/" + m[1]}, nil
		}
	}

rssOrWeb:
	canonicalURL := canonicalizeGenericURL(parsed)
	if isRSSURL(parsed) {
		sum := md5.Sum([]byte(canonicalURL))
		return types.ParsedURL{
			Platform:     types.PlatformRSS,
			ContentType:  types.ContentTypeFeed,
			PlatformID:   hex.EncodeToString(sum[:])[:16],
			CanonicalURL: canonicalURL,
		}, nil
	}

	sum := md5.Sum([]byte(canonicalURL))
	return types.ParsedURL{
		Platform:     types.PlatformWeb,
		ContentType:  types.ContentTypePost,
		PlatformID:   hex.EncodeToString(sum[:])[:16],
		CanonicalURL: canonicalURL,
	}, nil
}

func hostMatches(host string, domains ...string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func isReservedTwitterProfile(username string) bool {
	_, ok := twitterReservedProfiles[strings.ToLower(username)]
	return ok
}

func isRSSURL(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	if hostMatches(host, "hnrss.org") {
		return true
	}

	clean := strings.ToLower(strings.TrimSpace(parsed.Path))
	if clean == "" || clean == "/" {
		return false
	}

	trimmed := strings.Trim(clean, "/")
	base := path.Base(strings.TrimSuffix(clean, "/"))
	if base == "feed.xml" || base == "rss.xml" || base == "atom.xml" || strings.HasSuffix(base, ".rss") {
		return true
	}

	for _, segment := range strings.Split(trimmed, "/") {
		if slices.Contains(feedSegments, segment) {
			return true
		}
	}
	return false
}

func canonicalizeGenericURL(parsed *url.URL) string {
	canonical := *parsed
	canonical.Fragment = ""
	canonical.Scheme = strings.ToLower(canonical.Scheme)

	host := strings.ToLower(canonical.Hostname())
	port := canonical.Port()
	switch {
	case canonical.Scheme == "http" && port == "80":
		canonical.Host = host
	case canonical.Scheme == "https" && port == "443":
		canonical.Host = host
	case port != "":
		canonical.Host = host + ":" + port
	default:
		canonical.Host = host
	}

	query := canonical.Query()
	for key := range query {
		if shouldStripQueryParam(key) {
			query.Del(key)
		}
	}
	canonical.RawQuery = query.Encode()
	return canonical.String()
}

func shouldStripQueryParam(key string) bool {
	key = strings.ToLower(key)
	if strings.HasPrefix(key, "utm_") {
		return true
	}
	switch key {
	case "fbclid", "gclid", "dclid", "mc_cid", "mc_eid", "igshid":
		return true
	default:
		return false
	}
}

func normalizeURLHost(raw string, lowerHost string) string {
	withoutScheme := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	slash := strings.IndexByte(withoutScheme, '/')
	if slash < 0 {
		return lowerHost
	}
	return lowerHost + withoutScheme[slash:]
}
