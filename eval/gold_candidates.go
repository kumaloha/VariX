package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadGoldCandidates(path string) ([]GoldCandidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gold candidates: %w", err)
	}
	var candidates []GoldCandidate
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, fmt.Errorf("parse gold candidates: %w", err)
	}
	return candidates, nil
}

func LoadGoldCandidatesFromDir(dir string) ([]GoldCandidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read gold candidate dir: %w", err)
	}
	candidates := make([]GoldCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		candidate, err := LoadGoldCandidateFile(path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(candidate.SampleID) == "" {
			candidate.SampleID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func LoadGoldCandidateFile(path string) (GoldCandidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return GoldCandidate{}, fmt.Errorf("read gold candidate %s: %w", path, err)
	}
	var wrapped struct {
		SampleID string `json:"sample_id"`
		ID       string `json:"id"`
		Output   Output `json:"output"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return GoldCandidate{}, fmt.Errorf("parse gold candidate %s: %w", path, err)
	}
	if OutputHasGoldContent(wrapped.Output) {
		id := strings.TrimSpace(wrapped.SampleID)
		if id == "" {
			id = strings.TrimSpace(wrapped.ID)
		}
		return GoldCandidate{SampleID: id, Output: wrapped.Output}, nil
	}
	var output Output
	if err := json.Unmarshal(raw, &output); err != nil {
		return GoldCandidate{}, fmt.Errorf("parse gold candidate output %s: %w", path, err)
	}
	return GoldCandidate{Output: output}, nil
}

func OutputHasGoldContent(output Output) bool {
	return strings.TrimSpace(output.Summary) != "" ||
		len(output.Drivers) > 0 ||
		len(output.Targets) > 0 ||
		len(output.TransmissionPaths) > 0
}

func WriteGoldScorecardFile(path string, scorecard GoldScorecard) error {
	payload, err := json.MarshalIndent(scorecard, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gold scorecard: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write gold scorecard: %w", err)
	}
	return nil
}
