package normalize

import "testing"

func TestExtractURLs_DoesNotSwallowChineseSuffixAfterTCo(t *testing.T) {
	got := ExtractURLs("https://t.co/bCrRiLiPZd聊到，认同宝盛银行大宗商品主管的观点")
	if len(got) != 1 {
		t.Fatalf("len(ExtractURLs()) = %d, want 1", len(got))
	}
	if got[0] != "https://t.co/bCrRiLiPZd" {
		t.Fatalf("ExtractURLs()[0] = %q, want https://t.co/bCrRiLiPZd", got[0])
	}
}
