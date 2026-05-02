package twitter

import (
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestBuildAPIThreadContextUsesConversationScope(t *testing.T) {
	tweet := apiTweet{
		AuthorID:       "user-a",
		ConversationID: "100",
		ReferencedTweets: []apiReferencedTweet{
			{Type: "replied_to", ID: "99"},
		},
	}

	got := buildAPIThreadContext(tweet, map[string]string{"99": "user-a"})
	if got == nil {
		t.Fatal("buildAPIThreadContext() returned nil")
	}
	if got.ThreadID != "100" {
		t.Fatalf("ThreadID = %q, want 100", got.ThreadID)
	}
	if got.ThreadScope != types.ThreadScopeConversation {
		t.Fatalf("ThreadScope = %q, want %q", got.ThreadScope, types.ThreadScopeConversation)
	}
	if got.ParentExternalID != "99" {
		t.Fatalf("ParentExternalID = %q, want 99", got.ParentExternalID)
	}
	if !got.IsSelfThread {
		t.Fatal("IsSelfThread = false, want true")
	}
}

func TestBuildAPIThreadContextRootTweetStillUsesConversationScope(t *testing.T) {
	got := buildAPIThreadContext(apiTweet{
		AuthorID:       "user-a",
		ConversationID: "100",
	}, nil)
	if got == nil {
		t.Fatal("buildAPIThreadContext() returned nil")
	}
	if got.ThreadScope != types.ThreadScopeConversation {
		t.Fatalf("ThreadScope = %q, want %q", got.ThreadScope, types.ThreadScopeConversation)
	}
	if got.ParentExternalID != "" {
		t.Fatalf("ParentExternalID = %q, want empty", got.ParentExternalID)
	}
}

func TestBuildSyndicationThreadContextUsesSelfThreadScope(t *testing.T) {
	got := buildSyndicationThreadContext(syndicationPayload{
		SelfThread: &syndicationThread{IDStr: "100"},
	})
	if got == nil {
		t.Fatal("buildSyndicationThreadContext() returned nil")
	}
	if got.ThreadID != "100" {
		t.Fatalf("ThreadID = %q, want 100", got.ThreadID)
	}
	if got.ThreadScope != types.ThreadScopeSelfThread {
		t.Fatalf("ThreadScope = %q, want %q", got.ThreadScope, types.ThreadScopeSelfThread)
	}
	if !got.IsSelfThread {
		t.Fatal("IsSelfThread = false, want true")
	}
}
