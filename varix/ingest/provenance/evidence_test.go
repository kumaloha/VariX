package provenance

import (
	"testing"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestAppendEvidenceInitializesNilProvenance(t *testing.T) {
	got := AppendEvidence(nil, types.ProvenanceEvidence{
		Kind:   "k",
		Value:  "v",
		Weight: string(types.ConfidenceHigh),
	})
	if got == nil {
		t.Fatal("AppendEvidence() returned nil provenance")
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("len(Evidence) = %d, want 1", len(got.Evidence))
	}
}

func TestAppendEvidenceDedupesSameKindValueWeight(t *testing.T) {
	prov := &types.Provenance{}
	prov = AppendEvidence(prov, types.ProvenanceEvidence{Kind: "k", Value: "v", Weight: string(types.ConfidenceHigh)})
	prov = AppendEvidence(prov, types.ProvenanceEvidence{Kind: "k", Value: "v", Weight: string(types.ConfidenceHigh)})
	if len(prov.Evidence) != 1 {
		t.Fatalf("len(Evidence) = %d, want 1", len(prov.Evidence))
	}
}

func TestAppendEvidenceKeepsDifferentWeightDistinct(t *testing.T) {
	prov := &types.Provenance{}
	prov = AppendEvidence(prov, types.ProvenanceEvidence{Kind: "k", Value: "v", Weight: string(types.ConfidenceLow)})
	prov = AppendEvidence(prov, types.ProvenanceEvidence{Kind: "k", Value: "v", Weight: string(types.ConfidenceHigh)})
	if len(prov.Evidence) != 2 {
		t.Fatalf("len(Evidence) = %d, want 2", len(prov.Evidence))
	}
}
