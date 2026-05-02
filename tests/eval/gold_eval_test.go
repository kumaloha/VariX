package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGoldDatasetBatch1(t *testing.T) {
	for _, path := range batch1GoldDatasetPaths(t) {
		dataset, err := LoadGoldDataset(path)
		if err != nil {
			t.Fatalf("LoadGoldDataset(%s) error = %v", path, err)
		}
		if got := len(dataset.Samples); got != 9 {
			t.Fatalf("len(%s.Samples) = %d, want 9", path, got)
		}
		for _, sample := range dataset.Samples {
			if strings.TrimSpace(sample.Summary) == "" {
				t.Fatalf("%s sample %s summary is empty", path, sample.ID)
			}
			if len(sample.Drivers) == 0 {
				t.Fatalf("%s sample %s drivers are empty", path, sample.ID)
			}
			if len(sample.Targets) == 0 {
				t.Fatalf("%s sample %s targets are empty", path, sample.ID)
			}
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
		Version: "baseline",
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
		Version: "baseline",
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
	return batch1GoldDatasetPathNamed(t, "compile-gold-batch1-baseline.json")
}

func batch1GoldDatasetPaths(t *testing.T) []string {
	t.Helper()
	paths := []string{batch1GoldDatasetPathNamed(t, "compile-gold-batch1-baseline.json")}
	if path := optionalBatch1GoldDatasetPath(t, "compile-gold-batch1-refined.json"); path != "" {
		paths = append(paths, path)
	}
	return paths
}

func batch1GoldDatasetPathNamed(t *testing.T, name string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "eval", "gold", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	t.Fatalf("%s not found from %s upward", name, wd)
	return ""
}

func optionalBatch1GoldDatasetPath(t *testing.T, name string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "eval", "gold", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return ""
}
