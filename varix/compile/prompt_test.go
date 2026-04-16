package compile

import (
	"strings"
	"testing"
)

func TestBuildInstructionIncludesFatNodeSplitGuidance(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})

	for _, want := range []string{
		"如果一句话内部能自然改写成两步或以上因果链（A→B→C），优先拆成多个节点和边",
		"如果一句话同时混合已发生事实、当前判断和未来预测，优先拆开",
		"不要把整条宏观链压成一个“胖事实”节点",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}

func TestBuildInstructionMentionsQuarterAndYearPredictionWindows(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})

	for _, want := range []string{"下季度", "本季度", "明年"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}
