package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	c "github.com/kumaloha/VariX/varix/compile"
)

func runCompileGoldScore(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile gold-score", flag.ContinueOnError)
	fs.SetOutput(stderr)
	goldPath := fs.String("gold", "", "gold dataset JSON path")
	candidatePath := fs.String("candidate", "", "candidate JSON path with [{sample_id, output}]")
	candidateDir := fs.String("candidate-dir", "", "directory of candidate JSON reports named by sample id")
	outPath := fs.String("out", "", "optional output JSON path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*goldPath) == "" || (strings.TrimSpace(*candidatePath) == "" && strings.TrimSpace(*candidateDir) == "") {
		fmt.Fprintln(stderr, "usage: varix compile gold-score --gold <gold.json> (--candidate <candidate.json> | --candidate-dir <dir>)")
		return 2
	}
	if strings.TrimSpace(*candidatePath) != "" && strings.TrimSpace(*candidateDir) != "" {
		fmt.Fprintln(stderr, "compile gold-score accepts only one of --candidate or --candidate-dir")
		return 2
	}
	dataset, err := c.LoadGoldDataset(*goldPath)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	var candidates []c.GoldCandidate
	if strings.TrimSpace(*candidateDir) != "" {
		candidates, err = loadGoldCandidatesFromDir(*candidateDir)
	} else {
		candidates, err = loadGoldCandidates(*candidatePath)
	}
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	scorecard := c.ScoreGoldDataset(dataset, candidates)
	if strings.TrimSpace(*outPath) != "" {
		if err := writeGoldScorecardFile(*outPath, scorecard); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	return writeJSON(stdout, stderr, scorecard)
}

func loadGoldCandidates(path string) ([]c.GoldCandidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gold candidates: %w", err)
	}
	var candidates []c.GoldCandidate
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, fmt.Errorf("parse gold candidates: %w", err)
	}
	return candidates, nil
}

func loadGoldCandidatesFromDir(dir string) ([]c.GoldCandidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read gold candidate dir: %w", err)
	}
	candidates := make([]c.GoldCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		candidate, err := loadGoldCandidateFile(path)
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

func loadGoldCandidateFile(path string) (c.GoldCandidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return c.GoldCandidate{}, fmt.Errorf("read gold candidate %s: %w", path, err)
	}
	var wrapped struct {
		SampleID string   `json:"sample_id"`
		ID       string   `json:"id"`
		Output   c.Output `json:"output"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return c.GoldCandidate{}, fmt.Errorf("parse gold candidate %s: %w", path, err)
	}
	if outputHasGoldContent(wrapped.Output) {
		id := strings.TrimSpace(wrapped.SampleID)
		if id == "" {
			id = strings.TrimSpace(wrapped.ID)
		}
		return c.GoldCandidate{SampleID: id, Output: wrapped.Output}, nil
	}
	var output c.Output
	if err := json.Unmarshal(raw, &output); err != nil {
		return c.GoldCandidate{}, fmt.Errorf("parse gold candidate output %s: %w", path, err)
	}
	return c.GoldCandidate{Output: output}, nil
}

func outputHasGoldContent(output c.Output) bool {
	return strings.TrimSpace(output.Summary) != "" ||
		len(output.Drivers) > 0 ||
		len(output.Targets) > 0 ||
		len(output.TransmissionPaths) > 0
}

func writeGoldScorecardFile(path string, scorecard c.GoldScorecard) error {
	payload, err := json.MarshalIndent(scorecard, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gold scorecard: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write gold scorecard: %w", err)
	}
	return nil
}
