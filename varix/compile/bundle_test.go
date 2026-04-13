package compile

import (
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestBuildBundleCollectsRootAndLocalImages(t *testing.T) {
	raw := types.RawContent{
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
		AuthorName: "alice",
		AuthorID:   "u1",
		URL:        "https://x.com/alice/status/123",
		PostedAt:   time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC),
		Metadata:   types.RawMetadata{Thread: &types.ThreadContext{RootExternalID: "100"}},
		Quotes: []types.Quote{{
			Content: "quoted body",
		}},
		References: []types.Reference{{
			Content: "reference body",
			Attachments: []types.Attachment{{
				Type:       "image",
				StoredPath: "/tmp/ref.png",
			}},
		}},
		ThreadSegments: []types.ThreadSegment{{
			Position: 2,
			Content:  "thread body",
			Attachments: []types.Attachment{{
				Type:       "image",
				StoredPath: "/tmp/thread.png",
			}},
		}},
		Attachments: []types.Attachment{{
			Type:       "image",
			StoredPath: "/tmp/root.png",
		}},
	}

	got := BuildBundle(raw)
	if got.UnitID != "twitter:123" {
		t.Fatalf("UnitID = %q", got.UnitID)
	}
	if got.RootExternalID != "100" {
		t.Fatalf("RootExternalID = %q", got.RootExternalID)
	}
	if len(got.LocalImagePaths) != 3 {
		t.Fatalf("len(LocalImagePaths) = %d, want 3", len(got.LocalImagePaths))
	}
}

func TestBundleTextContextIncludesStructuredSections(t *testing.T) {
	b := Bundle{
		Content: "root body",
		Quotes: []types.Quote{{
			Content: "quoted body",
		}},
		References: []types.Reference{{
			Content: "reference body",
		}},
		ThreadSegments: []types.ThreadSegment{{
			Position: 2,
			Content:  "thread body",
		}},
		Attachments: []types.Attachment{{
			Type:       "video",
			Transcript: "spoken transcript",
		}},
	}

	got := b.TextContext()
	for _, want := range []string{
		"[ROOT CONTENT]\nroot body",
		"[QUOTE 1]\nquoted body",
		"[REFERENCE 1]\nreference body",
		"[THREAD 2]\nthread body",
		"[ATTACHMENT TRANSCRIPT 1]\nspoken transcript",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("TextContext() missing %q in %q", want, got)
		}
	}
}
