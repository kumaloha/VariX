package normalize

import "testing"

func TestExtractSpeakerFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{title: "付鹏：2026 大类资产怎么看", want: "付鹏"},
		{title: "【付鹏】全球宏观展望", want: "付鹏"},
		{title: "专访付鹏 全球宏观", want: "付鹏"},
		{title: "Random Video Title 2026", want: ""},
	}

	for _, tt := range tests {
		if got := ExtractSpeakerFromTitle(tt.title); got != tt.want {
			t.Fatalf("ExtractSpeakerFromTitle(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
