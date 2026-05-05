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

func TestBuildBundleSuppressesWebInfographicImagesForLongformBodies(t *testing.T) {
	raw := types.RawContent{
		Source:     "web",
		ExternalID: "article-1",
		Content:    strings.Repeat("正文", 1500),
		Attachments: []types.Attachment{
			{Type: "image", StoredPath: "/tmp/1.png"},
			{Type: "image", StoredPath: "/tmp/2.png"},
			{Type: "image", StoredPath: "/tmp/3.png"},
			{Type: "image", StoredPath: "/tmp/4.png"},
		},
	}
	got := BuildBundle(raw)
	if len(got.LocalImagePaths) != 0 {
		t.Fatalf("len(LocalImagePaths) = %d, want 0 for longform web article", len(got.LocalImagePaths))
	}
}

func TestBuildMergedBundleAddsIncludedSourcesAsReferences(t *testing.T) {
	primary := types.RawContent{
		Source:     "youtube",
		ExternalID: "QQOWQcnNmr0",
		Content:    "Buffett interview transcript",
		URL:        "https://www.youtube.com/watch?v=QQOWQcnNmr0",
	}
	meeting := types.RawContent{
		Source:     "youtube",
		ExternalID: "4VwLwtiuxVQ",
		Content:    "Berkshire annual meeting transcript",
		URL:        "https://www.youtube.com/watch?v=4VwLwtiuxVQ",
		Attachments: []types.Attachment{{
			Type:       "image",
			StoredPath: "/tmp/meeting-slide.png",
		}},
	}

	got := BuildMergedBundle(primary, []types.RawContent{meeting})
	if got.UnitID != "youtube:QQOWQcnNmr0" {
		t.Fatalf("UnitID = %q", got.UnitID)
	}
	if len(got.References) != 1 {
		t.Fatalf("len(References) = %d, want 1", len(got.References))
	}
	ref := got.References[0]
	if ref.Kind != "source_set" {
		t.Fatalf("reference kind = %q, want source_set", ref.Kind)
	}
	if ref.Source != "youtube" || ref.ExternalID != "4VwLwtiuxVQ" {
		t.Fatalf("reference source/id = %q/%q", ref.Source, ref.ExternalID)
	}
	for _, want := range []string{
		"[ROOT CONTENT]\nBuffett interview transcript",
		"[INCLUDED SOURCE 1 youtube:4VwLwtiuxVQ]\nBerkshire annual meeting transcript",
	} {
		if !strings.Contains(got.TextContext(), want) {
			t.Fatalf("TextContext() missing %q in %q", want, got.TextContext())
		}
	}
	if len(got.LocalImagePaths) != 1 || got.LocalImagePaths[0] != "/tmp/meeting-slide.png" {
		t.Fatalf("LocalImagePaths = %#v, want included source image path", got.LocalImagePaths)
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

func TestInferGraphRequirements(t *testing.T) {
	short := Bundle{Content: "short"}
	if got := InferGraphRequirements(short); got.MinNodes != 2 || got.MinEdges != 1 {
		t.Fatalf("short reqs = %#v", got)
	}
	mid := Bundle{Content: strings.Repeat("中", 3000)}
	if got := InferGraphRequirements(mid); got.MinNodes != 4 || got.MinEdges != 3 {
		t.Fatalf("mid reqs = %#v", got)
	}
	long := Bundle{Content: strings.Repeat("长", 9000)}
	if got := InferGraphRequirements(long); got.MinNodes != 6 || got.MinEdges != 5 {
		t.Fatalf("long reqs = %#v", got)
	}
}
