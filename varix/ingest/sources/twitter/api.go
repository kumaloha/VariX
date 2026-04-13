package twitter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/sources/httputil"
	"github.com/kumaloha/VariX/varix/ingest/types"
)

type APIHTTPClient struct {
	client *http.Client
	token  string
}

func NewAPIHTTPClient(client *http.Client, token string) *APIHTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &APIHTTPClient{
		client: client,
		token:  token,
	}
}

// apiMaxBytes is the upper bound for Twitter API JSON payloads (2 MB).
const apiMaxBytes = 2 << 20

type apiReferencedTweet struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type apiTweet struct {
	ID               string               `json:"id"`
	Text             string               `json:"text"`
	AuthorID         string               `json:"author_id"`
	CreatedAt        string               `json:"created_at"`
	ConversationID   string               `json:"conversation_id"`
	ReferencedTweets []apiReferencedTweet `json:"referenced_tweets"`
}

type apiUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type apiIncludedTweet struct {
	ID       string `json:"id"`
	AuthorID string `json:"author_id"`
}

type apiIncludes struct {
	Users  []apiUser          `json:"users"`
	Tweets []apiIncludedTweet `json:"tweets"`
}

type apiFetchResponse struct {
	Data     []apiTweet  `json:"data"`
	Includes apiIncludes `json:"includes"`
}

func buildAPIThreadContext(tweet apiTweet, includedAuthors map[string]string) *types.ThreadContext {
	if tweet.ConversationID == "" {
		return nil
	}

	tc := &types.ThreadContext{
		ThreadID:       tweet.ConversationID,
		ThreadScope:    types.ThreadScopeConversation,
		RootExternalID: tweet.ConversationID,
	}

	for _, ref := range tweet.ReferencedTweets {
		if ref.Type == "replied_to" {
			tc.ParentExternalID = ref.ID
			break
		}
	}

	if tc.ParentExternalID != "" {
		tc.ThreadIncomplete = true
		if parentAuthor, found := includedAuthors[tc.ParentExternalID]; found {
			tc.IsSelfThread = parentAuthor == tweet.AuthorID
		}
	}

	return tc
}

func (c *APIHTTPClient) FetchByID(ctx context.Context, tweetID string) ([]types.RawContent, error) {
	endpoint, err := url.Parse("https://api.twitter.com/2/tweets")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("ids", tweetID)
	query.Set("tweet.fields", "created_at,author_id,conversation_id,referenced_tweets")
	query.Set("expansions", "author_id,referenced_tweets.id")
	query.Set("user.fields", "username,name")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitter api fetch failed: status %d", resp.StatusCode)
	}

	if err := httputil.CheckContentLength(resp, apiMaxBytes); err != nil {
		return nil, err
	}

	var payload apiFetchResponse
	if err := httputil.DecodeJSONLimited(resp.Body, apiMaxBytes, &payload); err != nil {
		return nil, err
	}

	usernames := make(map[string]string, len(payload.Includes.Users))
	displayNames := make(map[string]string, len(payload.Includes.Users))
	for _, user := range payload.Includes.Users {
		usernames[user.ID] = user.Username
		displayNames[user.ID] = httputil.FirstString(user.Name, user.Username)
	}

	// Build a lookup from tweet ID to author ID using included tweets.
	includedAuthors := make(map[string]string, len(payload.Includes.Tweets))
	for _, t := range payload.Includes.Tweets {
		includedAuthors[t.ID] = t.AuthorID
	}

	items := make([]types.RawContent, 0, len(payload.Data))
	for _, tweet := range payload.Data {
		postedAt, err := time.Parse(time.RFC3339, tweet.CreatedAt)
		if err != nil {
			postedAt = time.Time{}
		}
		username := usernames[tweet.AuthorID]
		if username == "" {
			username = tweet.AuthorID
		}

		rc := types.RawContent{
			Source:     "twitter",
			ExternalID: tweet.ID,
			Content:    tweet.Text,
			AuthorName: displayNames[tweet.AuthorID],
			AuthorID:   tweet.AuthorID,
			URL:        fmt.Sprintf("https://x.com/%s/status/%s", username, tweet.ID),
			PostedAt:   postedAt.UTC(),
		}

		// Always attach thread context when conversation_id is present,
		// including for root tweets. This ensures downstream grouping
		// can find all records in a conversation by ThreadID.
		rc.Metadata.Thread = buildAPIThreadContext(tweet, includedAuthors)

		items = append(items, rc)
	}
	return items, nil
}
