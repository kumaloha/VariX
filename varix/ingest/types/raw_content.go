package types

import (
	"fmt"
	"strings"
	"time"
)

type RawContent struct {
	Source         string          `json:"source"`
	ExternalID     string          `json:"external_id"`
	Content        string          `json:"content"`
	AuthorName     string          `json:"author_name"`
	AuthorID       string          `json:"author_id,omitempty"`
	URL            string          `json:"url"`
	PostedAt       time.Time       `json:"posted_at"`
	Metadata       RawMetadata     `json:"metadata,omitempty"`
	MediaItems     []MediaItem     `json:"media_items,omitempty"` // Deprecated: use Attachments for new records.
	Quotes         []Quote         `json:"quotes,omitempty"`
	References     []Reference     `json:"references,omitempty"`
	ThreadSegments []ThreadSegment `json:"thread_segments,omitempty"`
	Attachments    []Attachment    `json:"attachments,omitempty"`
	Provenance     *Provenance     `json:"provenance,omitempty"`
}

// ExpandedText returns a richer text view for downstream analysis/reading.
// It keeps Content as-authored/stored, then appends quote bodies and
// attachment transcripts from structured fields.
func (r RawContent) ExpandedText() string {
	parts := make([]string, 0, 1+len(r.Quotes)+len(r.References)+len(r.Attachments))
	if trimmed := strings.TrimSpace(r.Content); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for i, quote := range r.Quotes {
		if trimmed := strings.TrimSpace(quote.Content); trimmed != "" {
			parts = append(parts, fmt.Sprintf("[引用正文#%d]\n%s", i+1, trimmed))
		}
	}
	for i, reference := range r.References {
		if trimmed := strings.TrimSpace(reference.Content); trimmed != "" {
			parts = append(parts, fmt.Sprintf("[参考正文#%d]\n%s", i+1, trimmed))
			continue
		}
		if trimmed := strings.TrimSpace(reference.URL); trimmed != "" {
			parts = append(parts, fmt.Sprintf("[参考链接#%d]\n%s", i+1, trimmed))
		}
	}
	for i, attachment := range r.Attachments {
		if trimmed := strings.TrimSpace(attachment.Transcript); trimmed != "" {
			parts = append(parts, fmt.Sprintf("[附件转写#%d]\n%s", i+1, trimmed))
		}
	}
	return strings.Join(parts, "\n\n")
}

type MediaItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Quote represents a referenced or quoted post within a RawContent record.
type Quote struct {
	Relation   string    `json:"relation"`
	AuthorName string    `json:"author_name,omitempty"`
	AuthorID   string    `json:"author_id,omitempty"`
	Platform   string    `json:"platform,omitempty"`
	ExternalID string    `json:"external_id,omitempty"`
	URL        string    `json:"url,omitempty"`
	Content    string    `json:"content"`
	PostedAt   time.Time `json:"posted_at,omitempty"`
}

// Reference represents a post/article link mentioned in the main body.
// Unlike Quote, it is not a platform-native quote/repost relationship.
type Reference struct {
	Kind          string       `json:"kind,omitempty"`
	Label         string       `json:"label,omitempty"`
	Source        string       `json:"source,omitempty"`
	Platform      string       `json:"platform,omitempty"`
	ExternalID    string       `json:"external_id,omitempty"`
	Content       string       `json:"content,omitempty"`
	AuthorName    string       `json:"author_name,omitempty"`
	AuthorID      string       `json:"author_id,omitempty"`
	URL           string       `json:"url"`
	PostedAt      time.Time    `json:"posted_at,omitempty"`
	Attachments   []Attachment `json:"attachments,omitempty"`
	QuoteURLs     []string     `json:"quote_urls,omitempty"`
	ReferenceURLs []string     `json:"reference_urls,omitempty"`
}

// ThreadSegment captures one post in an assembled self-thread view.
type ThreadSegment struct {
	ExternalID  string       `json:"external_id,omitempty"`
	URL         string       `json:"url,omitempty"`
	AuthorName  string       `json:"author_name,omitempty"`
	AuthorID    string       `json:"author_id,omitempty"`
	PostedAt    time.Time    `json:"posted_at,omitempty"`
	Position    int          `json:"position,omitempty"`
	Content     string       `json:"content,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment represents a media attachment with optional transcript.
type Attachment struct {
	Type                  string                 `json:"type"`
	URL                   string                 `json:"url"`
	PosterURL             string                 `json:"poster_url,omitempty"`
	Transcript            string                 `json:"transcript,omitempty"`
	Method                string                 `json:"method,omitempty"`
	TranscriptMethod      string                 `json:"transcript_method,omitempty"`
	TranscriptDiagnostics []TranscriptDiagnostic `json:"transcript_diagnostics,omitempty"`
}

// GetTranscriptMethod returns the canonical transcript method, preferring
// TranscriptMethod over the deprecated Method field for backward compatibility.
func (a Attachment) GetTranscriptMethod() string {
	if a.TranscriptMethod != "" {
		return a.TranscriptMethod
	}
	return a.Method
}

const (
	ThreadScopeConversation = "conversation"
	ThreadScopeSelfThread   = "self_thread"
)

// ThreadContext captures a post's position within a persisted thread scope.
// Persisted on RawContent.Metadata; DiscoveryItem.ThreadIDs is a discovery-only hint.
type ThreadContext struct {
	ThreadID         string `json:"thread_id,omitempty"`
	ThreadScope      string `json:"thread_scope,omitempty"`
	ParentExternalID string `json:"parent_external_id,omitempty"`
	RootExternalID   string `json:"root_external_id,omitempty"`
	ThreadPosition   *int   `json:"thread_position,omitempty"`
	ThreadIncomplete bool   `json:"thread_incomplete,omitempty"`
	IsSelfThread     bool   `json:"is_self_thread,omitempty"`
}

type RawMetadata struct {
	Thread   *ThreadContext    `json:"thread,omitempty"`
	Web      *WebMetadata      `json:"web,omitempty"`
	YouTube  *YouTubeMetadata  `json:"youtube,omitempty"`
	Bilibili *BilibiliMetadata `json:"bilibili,omitempty"`
	Twitter  *TwitterMetadata  `json:"twitter,omitempty"`
	Weibo    *WeiboMetadata    `json:"weibo,omitempty"`
}

type WebMetadata struct {
	Title           string `json:"title,omitempty"`
	SourceURL       string `json:"source_url,omitempty"`
	CanonicalURL    string `json:"canonical_url,omitempty"`
	YouTubeRedirect string `json:"youtube_redirect,omitempty"`
}

type YouTubeMetadata struct {
	Title                 string                 `json:"title,omitempty"`
	ChannelName           string                 `json:"channel_name,omitempty"`
	ChannelID             string                 `json:"channel_id,omitempty"`
	Description           string                 `json:"description,omitempty"`
	SourceLinks           []string               `json:"source_links,omitempty"`
	TranscriptMethod      string                 `json:"transcript_method,omitempty"`
	TranscriptDiagnostics []TranscriptDiagnostic `json:"transcript_diagnostics,omitempty"`
}

type BilibiliMetadata struct {
	Title                 string                 `json:"title,omitempty"`
	Uploader              string                 `json:"uploader,omitempty"`
	Description           string                 `json:"description,omitempty"`
	SourceLinks           []string               `json:"source_links,omitempty"`
	TranscriptMethod      string                 `json:"transcript_method,omitempty"`
	TranscriptDiagnostics []TranscriptDiagnostic `json:"transcript_diagnostics,omitempty"`
}

type TranscriptDiagnostic struct {
	Stage  string `json:"stage,omitempty"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type TwitterMetadata struct {
	Likes       int      `json:"likes,omitempty"`
	Retweets    int      `json:"retweets,omitempty"`
	Replies     int      `json:"replies,omitempty"`
	IsArticle   bool     `json:"is_article,omitempty"`
	ArticleURL  string   `json:"article_url,omitempty"`
	SourceLinks []string `json:"source_links,omitempty"`
}

type WeiboMetadata struct {
	IsRepost    bool   `json:"is_repost,omitempty"`
	OriginalURL string `json:"original_url,omitempty"`
}
