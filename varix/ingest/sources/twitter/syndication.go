package twitter

import (
	"regexp"
	"sort"
)

const syndicationEndpoint = "https://cdn.syndication.twimg.com/tweet-result"

var (
	bodyTwitterPost = regexp.MustCompile(`(?:twitter\.com|x\.com)/[\w]+/status/(\d+)`)
	bodyWeiboPost   = regexp.MustCompile(`weibo\.com/\d+/(\w+)|m\.weibo\.cn/(?:status|detail)/(\w+)`)
)

const (
	webBearerToken             = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
	tweetResultByRestIDQueryID = "tmhPpO5sDermwYmq3h034A"
)

// --- Typed payload structs for syndication JSON decoding ---

type syndicationUser struct {
	ScreenName string `json:"screen_name"`
	Name       string `json:"name"`
	IDStr      string `json:"id_str"`
}

type syndicationNote struct {
	NoteResults struct {
		Result struct {
			Text string `json:"text"`
		} `json:"result"`
	} `json:"note_tweet_results"`
}

type syndicationArticle struct {
	Title       string `json:"title"`
	PreviewText string `json:"preview_text"`
	RestID      string `json:"rest_id"`
}

type syndicationQuote struct {
	IDStr     string           `json:"id_str"`
	Text      string           `json:"text"`
	CreatedAt string           `json:"created_at"`
	User      syndicationUser  `json:"user"`
	NoteTweet *syndicationNote `json:"note_tweet,omitempty"`
}

type syndicationThread struct {
	IDStr string `json:"id_str"`
}

type syndicationVariant struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Bitrate     *int   `json:"bitrate,omitempty"`
}

type syndicationVideoInfo struct {
	Variants []syndicationVariant `json:"variants"`
}

type syndicationMedia struct {
	Type          string                `json:"type"`
	MediaURLHTTPS string                `json:"media_url_https"`
	MediaURL      string                `json:"media_url"`
	VideoInfo     *syndicationVideoInfo `json:"video_info,omitempty"`
}

type syndicationPayload struct {
	IDStr       string              `json:"id_str"`
	Text        string              `json:"text"`
	CreatedAt   string              `json:"created_at"`
	User        syndicationUser     `json:"user"`
	Article     *syndicationArticle `json:"article,omitempty"`
	NoteTweet   *syndicationNote    `json:"note_tweet,omitempty"`
	QuotedTweet *syndicationQuote   `json:"quoted_tweet,omitempty"`
	SelfThread  *syndicationThread  `json:"self_thread,omitempty"`
	Media       []syndicationMedia  `json:"mediaDetails,omitempty"`
	Likes       int                 `json:"favorite_count"`
	Retweets    int                 `json:"retweet_count"`
	Replies     int                 `json:"conversation_count"`
	Tombstone   any                 `json:"tombstone,omitempty"`
	NotFound    bool                `json:"notFound,omitempty"`
}

// selectPreferredMP4 picks a mid-bitrate MP4 variant from the list.
// It filters to video/mp4, sorts by bitrate ascending, and returns the
// middle entry. Returns "" if no MP4 variants are available.

func selectPreferredMP4(variants []syndicationVariant) string {
	var mp4s []syndicationVariant
	for _, v := range variants {
		if v.ContentType == "video/mp4" {
			mp4s = append(mp4s, v)
		}
	}
	if len(mp4s) == 0 {
		return ""
	}
	sort.Slice(mp4s, func(i, j int) bool {
		bi, bj := 0, 0
		if mp4s[i].Bitrate != nil {
			bi = *mp4s[i].Bitrate
		}
		if mp4s[j].Bitrate != nil {
			bj = *mp4s[j].Bitrate
		}
		return bi < bj
	})
	return mp4s[len(mp4s)/2].URL
}
