package verify

import "testing"

func TestVerifyDeclarationsDetailedClassifiesSourceBackedDeclarations(t *testing.T) {
	got := verifyDeclarationsDetailed([]Declaration{{
		ID:          "decl-1",
		Speaker:     "Greg Abel",
		Statement:   "伯克希尔会等待市场错配",
		SourceQuote: "there will be dislocations in markets",
		Evidence:    []string{"现金和短债约3800亿美元"},
	}})

	if len(got) != 1 {
		t.Fatalf("verifyDeclarationsDetailed() = %#v, want one result", got)
	}
	if got[0].Status != DeclarationVerificationProved {
		t.Fatalf("Status = %q, want proved", got[0].Status)
	}
	if got[0].Speaker != "Greg Abel" || got[0].Statement != "伯克希尔会等待市场错配" {
		t.Fatalf("verification = %#v, want speaker and statement copied", got[0])
	}
}

func TestVerifyDeclarationsDetailedMarksUnsourcedDeclarationsInferredOnly(t *testing.T) {
	got := verifyDeclarationsDetailed([]Declaration{{
		ID:        "decl-2",
		Statement: "伯克希尔不会被迫部署资本",
	}})

	if len(got) != 1 {
		t.Fatalf("verifyDeclarationsDetailed() = %#v, want one result", got)
	}
	if got[0].Status != DeclarationVerificationInferredOnly {
		t.Fatalf("Status = %q, want inferred_only", got[0].Status)
	}
}
