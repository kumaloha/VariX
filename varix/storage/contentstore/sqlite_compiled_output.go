package contentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
)

func (s *SQLiteStore) UpsertCompiledOutput(ctx context.Context, record compile.Record) error {
	if record.Source == "" || record.ExternalID == "" || record.Model == "" {
		return fmt.Errorf("invalid compiled output")
	}
	if err := record.Output.Validate(); err != nil {
		return err
	}
	record.CompiledAt = normalizeRecordedTime(record.CompiledAt)
	payload, err := marshalStoredCompileRecord(record)
	if err != nil {
		return err
	}
	now := currentSQLiteTimestamp()
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO compiled_outputs(platform, external_id, root_external_id, model, payload_json, compiled_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   root_external_id = excluded.root_external_id,
		   model = excluded.model,
		   payload_json = excluded.payload_json,
		   compiled_at = excluded.compiled_at,
		   updated_at = excluded.updated_at`,
		record.Source,
		record.ExternalID,
		record.RootExternalID,
		record.Model,
		string(payload),
		record.CompiledAt.UTC().Format(time.RFC3339Nano),
		now,
	)
	if err != nil {
		return err
	}
	if subgraph, err := graphmodel.FromCompileRecord(record); err != nil {
		return err
	} else if err := s.UpsertContentSubgraph(ctx, subgraph); err != nil {
		return err
	}
	if !record.Output.Verification.IsZero() {
		model := strings.TrimSpace(record.Output.Verification.Model)
		if model == "" {
			model = record.Model
		}
		verifiedAt := record.Output.Verification.VerifiedAt
		if verifiedAt.IsZero() {
			verifiedAt = normalizeRecordedTime(verifiedAt)
		}
		if err := s.UpsertVerificationResult(ctx, compile.VerificationRecord{
			UnitID:         record.UnitID,
			Source:         record.Source,
			ExternalID:     record.ExternalID,
			RootExternalID: record.RootExternalID,
			Model:          model,
			Verification:   record.Output.Verification,
			VerifiedAt:     verifiedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) GetCompiledOutput(ctx context.Context, platform, externalID string) (compile.Record, error) {
	var payload string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json FROM compiled_outputs WHERE platform = ? AND external_id = ?`,
		platform,
		externalID,
	).Scan(&payload)
	if err != nil {
		return compile.Record{}, err
	}
	var record compile.Record
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return compile.Record{}, err
	}
	return record, nil
}

func marshalStoredCompileRecord(record compile.Record) ([]byte, error) {
	type storedOutput struct {
		Summary            string                     `json:"summary,omitempty"`
		Drivers            []string                   `json:"drivers,omitempty"`
		Targets            []string                   `json:"targets,omitempty"`
		TransmissionPaths  []compile.TransmissionPath `json:"transmission_paths,omitempty"`
		EvidenceNodes      []string                   `json:"evidence_nodes,omitempty"`
		ExplanationNodes   []string                   `json:"explanation_nodes,omitempty"`
		SupplementaryNodes []string                   `json:"supplementary_nodes,omitempty"`
		Graph              compile.ReasoningGraph     `json:"graph,omitempty"`
		Details            compile.HiddenDetails      `json:"details,omitempty"`
		Topics             []string                   `json:"topics,omitempty"`
		Confidence         string                     `json:"confidence,omitempty"`
	}
	type storedRecord struct {
		UnitID         string                `json:"unit_id"`
		Source         string                `json:"source"`
		ExternalID     string                `json:"external_id"`
		RootExternalID string                `json:"root_external_id,omitempty"`
		Model          string                `json:"model"`
		Metrics        compile.RecordMetrics `json:"metrics,omitempty"`
		Output         storedOutput          `json:"output"`
		CompiledAt     time.Time             `json:"compiled_at"`
	}
	return json.Marshal(storedRecord{
		UnitID:         record.UnitID,
		Source:         record.Source,
		ExternalID:     record.ExternalID,
		RootExternalID: record.RootExternalID,
		Model:          record.Model,
		Metrics:        record.Metrics,
		Output: storedOutput{
			Summary:            record.Output.Summary,
			Drivers:            record.Output.Drivers,
			Targets:            record.Output.Targets,
			TransmissionPaths:  record.Output.TransmissionPaths,
			EvidenceNodes:      record.Output.EvidenceNodes,
			ExplanationNodes:   record.Output.ExplanationNodes,
			SupplementaryNodes: record.Output.SupplementaryNodes,
			Graph:              record.Output.Graph,
			Details:            record.Output.Details,
			Topics:             record.Output.Topics,
			Confidence:         record.Output.Confidence,
		},
		CompiledAt: record.CompiledAt,
	})
}
