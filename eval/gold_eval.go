package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type GoldDataset struct {
	Version string       `json:"version"`
	Scope   string       `json:"scope,omitempty"`
	Domain  string       `json:"domain,omitempty"`
	Samples []GoldSample `json:"samples"`
}

type GoldSample struct {
	ID      string   `json:"id"`
	Title   string   `json:"title,omitempty"`
	URL     string   `json:"url,omitempty"`
	Summary string   `json:"summary"`
	Drivers []string `json:"drivers,omitempty"`
	Targets []string `json:"targets,omitempty"`
}

type GoldEvaluationSection struct {
	Name        string   `json:"name"`
	SampleCount int      `json:"sample_count"`
	Metrics     []string `json:"metrics"`
}

type GoldEvaluationReport struct {
	DatasetVersion string                `json:"dataset_version"`
	SampleCount    int                   `json:"sample_count"`
	Summary        GoldEvaluationSection `json:"summary"`
	NodeRecallType GoldEvaluationSection `json:"node_recall_type"`
	ReasoningEdges GoldEvaluationSection `json:"reasoning_edges"`
	Drivers        GoldEvaluationSection `json:"drivers"`
	Targets        GoldEvaluationSection `json:"targets"`
}

func LoadGoldDataset(path string) (GoldDataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return GoldDataset{}, fmt.Errorf("read gold dataset: %w", err)
	}
	var dataset GoldDataset
	if err := json.Unmarshal(raw, &dataset); err != nil {
		return GoldDataset{}, fmt.Errorf("parse gold dataset: %w", err)
	}
	if err := dataset.Validate(); err != nil {
		return GoldDataset{}, err
	}
	return dataset, nil
}

func (d GoldDataset) Validate() error {
	if strings.TrimSpace(d.Version) == "" {
		return fmt.Errorf("gold dataset version is required")
	}
	if len(d.Samples) == 0 {
		return fmt.Errorf("gold dataset must contain at least one sample")
	}
	for i, sample := range d.Samples {
		if strings.TrimSpace(sample.ID) == "" {
			return fmt.Errorf("gold dataset sample[%d] id is required", i)
		}
		if strings.TrimSpace(sample.Summary) == "" {
			return fmt.Errorf("gold dataset sample[%d] summary is required", i)
		}
		if len(sample.Drivers) == 0 {
			return fmt.Errorf("gold dataset sample[%d] drivers are required", i)
		}
		if len(sample.Targets) == 0 {
			return fmt.Errorf("gold dataset sample[%d] targets are required", i)
		}
		if err := validateNonEmptyStrings(fmt.Sprintf("gold dataset sample[%d] drivers", i), sample.Drivers); err != nil {
			return err
		}
		if err := validateNonEmptyStrings(fmt.Sprintf("gold dataset sample[%d] targets", i), sample.Targets); err != nil {
			return err
		}
	}
	return nil
}

func BuildGoldEvaluationReport(dataset GoldDataset) GoldEvaluationReport {
	sampleCount := len(dataset.Samples)
	return GoldEvaluationReport{
		DatasetVersion: dataset.Version,
		SampleCount:    sampleCount,
		Summary: GoldEvaluationSection{
			Name:        "summary",
			SampleCount: sampleCount,
			Metrics:     []string{"quality"},
		},
		NodeRecallType: GoldEvaluationSection{
			Name:        "node_recall_type",
			SampleCount: sampleCount,
			Metrics:     []string{"recall", "typing_accuracy"},
		},
		ReasoningEdges: GoldEvaluationSection{
			Name:        "reasoning_edges",
			SampleCount: sampleCount,
			Metrics:     []string{"edge_quality"},
		},
		Drivers: GoldEvaluationSection{
			Name:        "drivers",
			SampleCount: sampleCount,
			Metrics:     []string{"extraction_quality"},
		},
		Targets: GoldEvaluationSection{
			Name:        "targets",
			SampleCount: sampleCount,
			Metrics:     []string{"extraction_quality"},
		},
	}
}

func validateNonEmptyStrings(field string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s[%d] must not be empty", field, i)
		}
	}
	return nil
}
