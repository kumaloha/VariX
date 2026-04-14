package compile

import (
	"strings"
	"testing"
)

func TestBuildInstructionMentionsQuarterAndYearPredictionWindows(t *testing.T) {
	instruction := BuildInstruction(GraphRequirements{MinNodes: 2, MinEdges: 1})
	for _, want := range []string{"下季度", "本季度", "明年"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("BuildInstruction() missing %q in %q", want, instruction)
		}
	}
}
