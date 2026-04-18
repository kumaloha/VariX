package compile

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGoldDatasetBatch1(t *testing.T) {
	dataset, err := LoadGoldDataset(batch1GoldDatasetPath(t))
	if err != nil {
		t.Fatalf("LoadGoldDataset() error = %v", err)
	}
	if got := len(dataset.Samples); got != 9 {
		t.Fatalf("len(dataset.Samples) = %d, want 9", got)
	}
	for _, sample := range dataset.Samples {
		if strings.TrimSpace(sample.Summary) == "" {
			t.Fatalf("sample %s summary is empty", sample.ID)
		}
		if len(sample.Drivers) == 0 {
			t.Fatalf("sample %s drivers are empty", sample.ID)
		}
		if len(sample.Targets) == 0 {
			t.Fatalf("sample %s targets are empty", sample.ID)
		}
	}
}

func TestBuildGoldEvaluationReportBatch1(t *testing.T) {
	dataset, err := LoadGoldDataset(batch1GoldDatasetPath(t))
	if err != nil {
		t.Fatalf("LoadGoldDataset() error = %v", err)
	}
	report := BuildGoldEvaluationReport(dataset)
	if report.DatasetVersion != dataset.Version {
		t.Fatalf("report.DatasetVersion = %q, want %q", report.DatasetVersion, dataset.Version)
	}
	if report.SampleCount != len(dataset.Samples) {
		t.Fatalf("report.SampleCount = %d, want %d", report.SampleCount, len(dataset.Samples))
	}
	for _, section := range []GoldEvaluationSection{
		report.Summary,
		report.NodeRecallType,
		report.ReasoningEdges,
		report.Drivers,
		report.Targets,
	} {
		if section.Name == "" {
			t.Fatalf("section name is empty: %#v", section)
		}
		if section.SampleCount != len(dataset.Samples) {
			t.Fatalf("section %s sample_count = %d, want %d", section.Name, section.SampleCount, len(dataset.Samples))
		}
		if len(section.Metrics) == 0 {
			t.Fatalf("section %s metrics are empty", section.Name)
		}
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(report) error = %v", err)
	}
	t.Logf("batch1 gold evaluation report: %s", raw)
}

func TestGoldDatasetValidateRejectsBlankDriverOrTarget(t *testing.T) {
	dataset := GoldDataset{
		Version: "v1",
		Samples: []GoldSample{
			{
				ID:      "G01",
				Summary: "summary",
				Drivers: []string{"driver", " "},
				Targets: []string{"target"},
			},
		},
	}
	if err := dataset.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want blank driver rejection")
	}

	dataset = GoldDataset{
		Version: "v1",
		Samples: []GoldSample{
			{
				ID:      "G01",
				Summary: "summary",
				Drivers: []string{"driver"},
				Targets: []string{" ", "target"},
			},
		},
	}
	if err := dataset.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want blank target rejection")
	}
}

func batch1GoldDatasetPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "data", "gold", "compile-gold-batch1-v1.json")
}
