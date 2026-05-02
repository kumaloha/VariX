package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kumaloha/VariX/eval"
)

func main() {
	fs := flag.NewFlagSet("goldscore", flag.ExitOnError)
	goldPath := fs.String("gold", "", "gold dataset JSON path")
	candidatePath := fs.String("candidate", "", "candidate JSON path with [{sample_id, output}]")
	candidateDir := fs.String("candidate-dir", "", "directory of candidate JSON reports named by sample id")
	outPath := fs.String("out", "", "optional output JSON path")
	_ = fs.Parse(os.Args[1:])

	if strings.TrimSpace(*goldPath) == "" || (strings.TrimSpace(*candidatePath) == "" && strings.TrimSpace(*candidateDir) == "") {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/goldscore --gold <gold.json> (--candidate <candidate.json> | --candidate-dir <dir>)")
		os.Exit(2)
	}
	if strings.TrimSpace(*candidatePath) != "" && strings.TrimSpace(*candidateDir) != "" {
		fmt.Fprintln(os.Stderr, "goldscore accepts only one of --candidate or --candidate-dir")
		os.Exit(2)
	}

	dataset, err := eval.LoadGoldDataset(*goldPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var candidates []eval.GoldCandidate
	if strings.TrimSpace(*candidateDir) != "" {
		candidates, err = eval.LoadGoldCandidatesFromDir(*candidateDir)
	} else {
		candidates, err = eval.LoadGoldCandidates(*candidatePath)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	scorecard := eval.ScoreGoldDataset(dataset, candidates)
	if strings.TrimSpace(*outPath) != "" {
		if err := eval.WriteGoldScorecardFile(*outPath, scorecard); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(scorecard); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
