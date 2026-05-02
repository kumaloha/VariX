package verify

import (
	"testing"
	"time"
)

func TestClassifyVerificationTimeBucketUsesPostedAtAndExtractedTiming(t *testing.T) {
	postedAt := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	bundle := Bundle{PostedAt: postedAt}

	tests := []struct {
		name string
		node GraphNode
		want verificationTimeBucket
	}{
		{
			name: "occurred before post is realized",
			node: GraphNode{ID: "n1", Text: "已发生事件", OccurredAt: postedAt.Add(-24 * time.Hour)},
			want: verificationBucketRealized,
		},
		{
			name: "future due is future",
			node: GraphNode{ID: "n2", Text: "未来会发生", PredictionDueAt: postedAt.Add(24 * time.Hour)},
			want: verificationBucketFuture,
		},
		{
			name: "expired due becomes realized",
			node: GraphNode{ID: "n3", Text: "到期后可判定", PredictionDueAt: postedAt.Add(-time.Hour)},
			want: verificationBucketRealized,
		},
		{
			name: "condition nodes always route to future extractor",
			node: GraphNode{ID: "n4", Kind: NodeExplicitCondition, Text: "若流动性继续收紧"},
			want: verificationBucketFuture,
		},
		{
			name: "future language without concrete timestamps stays future",
			node: GraphNode{ID: "n5", Text: "未来几个月风险会扩大"},
			want: verificationBucketFuture,
		},
		{
			name: "present-state conclusion with chinese tense cues is realized",
			node: GraphNode{ID: "n6", Kind: NodeConclusion, Text: "美股已经进入超买"},
			want: verificationBucketRealized,
		},
		{
			name: "english current-state judgment is realized",
			node: GraphNode{ID: "n8", Kind: NodeConclusion, Text: "US equity market rally into overbought territory"},
			want: verificationBucketRealized,
		},
		{
			name: "generic conclusion without tense cues stays undetermined",
			node: GraphNode{ID: "n9", Kind: NodeConclusion, Form: NodeFormJudgment, Function: NodeFunctionClaim, Text: "结论B"},
			want: verificationBucketUndetermined,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyVerificationTimeBucket(bundle, tc.node)
			if got != tc.want {
				t.Fatalf("classifyVerificationTimeBucket(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
