package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGoldCandidateIOReadsCandidateDirAndWritesScorecard(t *testing.T) {
	tmp := t.TempDir()
	candidateDir := filepath.Join(tmp, "candidates")
	outPath := filepath.Join(tmp, "scorecard.json")
	if err := os.Mkdir(candidateDir, 0o755); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	dataset := GoldDataset{
		Version: "test-baseline",
		Samples: []GoldSample{{
			ID:      "G01",
			Summary: "资金流入硬资产以对冲货币贬值",
			Drivers: []string{"央行扩表压低实际利率"},
			Targets: []string{"资金流入硬资产"},
		}},
	}
	report := struct {
		UnitID string `json:"unit_id"`
		Output Output `json:"output"`
	}{
		UnitID: "twitter:1",
		Output: Output{
			Summary: "实际利率为负导致资金买入黄金",
			Drivers: []string{"实际利率下降"},
			Targets: []string{"资金买入黄金"},
		},
	}
	writeEvalTestJSONFile(t, filepath.Join(candidateDir, "G01.json"), report)

	candidates, err := LoadGoldCandidatesFromDir(candidateDir)
	if err != nil {
		t.Fatalf("LoadGoldCandidatesFromDir() error = %v", err)
	}
	scorecard := ScoreGoldDataset(dataset, candidates)
	if scorecard.SampleCount != 1 || len(scorecard.Samples) != 1 {
		t.Fatalf("scorecard = %#v, want one scored sample", scorecard)
	}
	if scorecard.Samples[0].ID != "G01" {
		t.Fatalf("sample id = %q, want G01", scorecard.Samples[0].ID)
	}
	if len(scorecard.Samples[0].ReviewItems) == 0 {
		t.Fatalf("scorecard missing review items: %#v", scorecard)
	}

	if err := WriteGoldScorecardFile(outPath, scorecard); err != nil {
		t.Fatalf("WriteGoldScorecardFile() error = %v", err)
	}
	rawOut, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("os.ReadFile(outPath) error = %v", err)
	}
	var fileOut GoldScorecard
	if err := json.Unmarshal(rawOut, &fileOut); err != nil {
		t.Fatalf("json.Unmarshal(outPath) error = %v; raw=%s", err, string(rawOut))
	}
	if fileOut.DatasetVersion != "test-baseline" {
		t.Fatalf("file scorecard version = %q, want test-baseline", fileOut.DatasetVersion)
	}
}

func writeEvalTestJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
}
