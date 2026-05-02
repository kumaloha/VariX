package contentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/model"
)

type ParadigmEvidenceLink struct {
	ParadigmID   string `json:"paradigm_id"`
	EventGraphID string `json:"event_graph_id,omitempty"`
	SubgraphID   string `json:"subgraph_id,omitempty"`
}

type ParadigmRecord struct {
	ParadigmID                string              `json:"paradigm_id"`
	UserID                    string              `json:"user_id"`
	DriverSubject             string              `json:"driver_subject"`
	TargetSubject             string              `json:"target_subject"`
	TimeBucket                string              `json:"time_bucket"`
	SupportingSubgraphIDs     []string            `json:"supporting_subgraph_ids,omitempty"`
	SupportingSubgraphCount   int                 `json:"supporting_subgraph_count"`
	SupportingEventGraphIDs   []string            `json:"supporting_event_graph_ids,omitempty"`
	SupportingEventGraphCount int                 `json:"supporting_event_graph_count"`
	TraceabilityMap           map[string][]string `json:"traceability_map,omitempty"`
	SuccessCount              int                 `json:"success_count"`
	FailureCount              int                 `json:"failure_count"`
	CredibilityScore          float64             `json:"credibility_score"`
	CredibilityState          string              `json:"credibility_state"`
	RepresentativeChanges     []string            `json:"representative_changes,omitempty"`
	UpdatedAt                 string              `json:"updated_at"`
}

func (s *SQLiteStore) RunParadigmProjection(ctx context.Context, userID string, now time.Time) ([]ParadigmRecord, error) {
	var err error
	userID, err = normalizeRequiredUserID(userID)
	if err != nil {
		return nil, err
	}
	now = normalizeNow(now)
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? ORDER BY source_platform ASC, source_external_id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type state struct {
		driver       string
		target       string
		bucket       string
		subgraphs    []string
		eventGraphs  []string
		changes      []string
		traceability map[string][]string
		success      int
		failure      int
	}
	byKey := map[string]*state{}
	canonicalCache := map[string]string{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph model.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory_content_graph payload: %w", err)
		}
		drivers := make([]model.ContentNode, 0)
		targets := make([]model.ContentNode, 0)
		for _, node := range subgraph.Nodes {
			if !node.IsPrimary {
				continue
			}
			switch node.GraphRole {
			case model.GraphRoleDriver:
				drivers = append(drivers, node)
			case model.GraphRoleTarget:
				targets = append(targets, node)
			}
		}
		for _, driver := range drivers {
			for _, target := range targets {
				driverSubject, err := s.resolveCanonicalGraphNodeSubject(ctx, driver, canonicalCache)
				if err != nil {
					return nil, err
				}
				targetSubject, err := s.resolveCanonicalGraphNodeSubject(ctx, target, canonicalCache)
				if err != nil {
					return nil, err
				}
				bucket := normalizedEventBucket(driver.TimeBucket, target.TimeBucket, deriveEventBucket(driver), deriveEventBucket(target), "timeless")
				key := normalizeCanonicalAlias(driverSubject) + "|" + normalizeCanonicalAlias(targetSubject) + "|" + bucket
				st := byKey[key]
				if st == nil {
					st = &state{
						driver:       driverSubject,
						target:       targetSubject,
						bucket:       bucket,
						traceability: map[string][]string{},
					}
					byKey[key] = st
				}
				st.subgraphs = append(st.subgraphs, subgraph.ID)
				st.eventGraphs = append(st.eventGraphs, buildEventGraphID(userID, "driver", driverSubject, bucket))
				st.changes = append(st.changes, strings.TrimSpace(driver.ChangeText), strings.TrimSpace(target.ChangeText))
				st.traceability[subgraph.ID] = uniqueStrings(append(st.traceability[subgraph.ID], driver.ID, target.ID))
				switch target.VerificationStatus {
				case model.VerificationProved:
					st.success++
				case model.VerificationDisproved:
					st.failure++
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]ParadigmRecord, 0, len(byKey))
	keepParadigmIDs := make([]string, 0, len(byKey))
	for _, st := range byKey {
		subgraphs := uniqueStrings(st.subgraphs)
		eventGraphs := uniqueStrings(st.eventGraphs)
		changes := uniqueStrings(filterNonBlank(st.changes))
		score := paradigmCredibilityScore(st.success, st.failure)
		record := ParadigmRecord{
			ParadigmID:                buildParadigmID(userID, st.driver, st.target, st.bucket),
			UserID:                    userID,
			DriverSubject:             st.driver,
			TargetSubject:             st.target,
			TimeBucket:                st.bucket,
			SupportingSubgraphIDs:     subgraphs,
			SupportingSubgraphCount:   len(subgraphs),
			SupportingEventGraphIDs:   eventGraphs,
			SupportingEventGraphCount: len(eventGraphs),
			TraceabilityMap:           st.traceability,
			SuccessCount:              st.success,
			FailureCount:              st.failure,
			CredibilityScore:          score,
			CredibilityState:          paradigmCredibilityState(score, st.success, st.failure),
			RepresentativeChanges:     changes,
			UpdatedAt:                 now.UTC().Format(time.RFC3339),
		}
		if err := s.upsertParadigm(ctx, record); err != nil {
			return nil, err
		}
		if err := s.replaceParadigmEvidenceLinks(ctx, record); err != nil {
			return nil, err
		}
		out = append(out, record)
		keepParadigmIDs = append(keepParadigmIDs, record.ParadigmID)
	}
	if err := s.deleteStaleParadigms(ctx, userID, keepParadigmIDs); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DriverSubject != out[j].DriverSubject {
			return out[i].DriverSubject < out[j].DriverSubject
		}
		if out[i].TargetSubject != out[j].TargetSubject {
			return out[i].TargetSubject < out[j].TargetSubject
		}
		return out[i].TimeBucket < out[j].TimeBucket
	})
	return out, nil
}

func (s *SQLiteStore) deleteStaleParadigms(ctx context.Context, userID string, keepParadigmIDs []string) error {
	userID = strings.TrimSpace(userID)
	keep := uniqueStrings(filterNonBlank(keepParadigmIDs))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{userID}
	query := `SELECT paradigm_id FROM paradigms WHERE user_id = ?`
	if len(keep) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(keep)), ",")
		query += ` AND paradigm_id NOT IN (` + placeholders + `)`
		for _, id := range keep {
			args = append(args, id)
		}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	staleIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		staleIDs = append(staleIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range staleIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM paradigm_evidence_links WHERE paradigm_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM paradigms WHERE paradigm_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListParadigmsBySubject(ctx context.Context, userID, subject string) ([]ParadigmRecord, error) {
	subject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM paradigms WHERE user_id = ? AND (driver_subject = ? OR target_subject = ?) ORDER BY driver_subject ASC, target_subject ASC, time_bucket ASC`, strings.TrimSpace(userID), strings.TrimSpace(subject), strings.TrimSpace(subject))
	if err != nil {
		return nil, err
	}
	return decodePayloadRows[ParadigmRecord](rows, "paradigm")
}

func (s *SQLiteStore) ListParadigms(ctx context.Context, userID string) ([]ParadigmRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM paradigms WHERE user_id = ? ORDER BY driver_subject ASC, target_subject ASC, time_bucket ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	return decodePayloadRows[ParadigmRecord](rows, "paradigm")
}

func (s *SQLiteStore) upsertParadigm(ctx context.Context, record ParadigmRecord) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	updatedAt, err := time.Parse(time.RFC3339, record.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO paradigms(paradigm_id, user_id, driver_subject, target_subject, time_bucket, payload_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(paradigm_id) DO UPDATE SET
		  payload_json = excluded.payload_json,
		  updated_at = excluded.updated_at`,
		record.ParadigmID,
		record.UserID,
		record.DriverSubject,
		record.TargetSubject,
		record.TimeBucket,
		string(payload),
		updatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func buildParadigmID(userID, driver, target, bucket string) string {
	parts := []string{strings.TrimSpace(userID), normalizeCanonicalAlias(driver), normalizeCanonicalAlias(target), strings.TrimSpace(bucket)}
	return strings.Join(parts, ":")
}

func paradigmCredibilityScore(success, failure int) float64 {
	score := float64(success) - 1.5*float64(failure)
	if score < 0 {
		return 0
	}
	return score
}

func paradigmCredibilityState(score float64, success, failure int) string {
	switch {
	case failure > success && failure > 0:
		return "degraded"
	case score >= 2:
		return "explicit"
	case score > 0:
		return "candidate"
	default:
		return "latent"
	}
}

func filterNonBlank(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func (s *SQLiteStore) replaceParadigmEvidenceLinks(ctx context.Context, record ParadigmRecord) error {
	if strings.TrimSpace(record.ParadigmID) == "" {
		return fmt.Errorf("paradigm id is required")
	}
	now := currentSQLiteTimestamp()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM paradigm_evidence_links WHERE paradigm_id = ?`, record.ParadigmID); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, subgraphID := range record.SupportingSubgraphIDs {
		key := record.ParadigmID + "|" + subgraphID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT INTO paradigm_evidence_links(paradigm_id, event_graph_id, subgraph_id, created_at) VALUES (?, ?, ?, ?)`, record.ParadigmID, "", subgraphID, now); err != nil {
			return err
		}
	}
	for _, eventGraphID := range record.SupportingEventGraphIDs {
		key := record.ParadigmID + "|event|" + eventGraphID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT INTO paradigm_evidence_links(paradigm_id, event_graph_id, subgraph_id, created_at) VALUES (?, ?, ?, ?)`, record.ParadigmID, eventGraphID, "", now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListParadigmEvidenceLinks(ctx context.Context, paradigmID string) ([]ParadigmEvidenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT paradigm_id, event_graph_id, subgraph_id FROM paradigm_evidence_links WHERE paradigm_id = ? ORDER BY event_graph_id ASC, subgraph_id ASC`, strings.TrimSpace(paradigmID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ParadigmEvidenceLink, 0)
	for rows.Next() {
		var link ParadigmEvidenceLink
		if err := rows.Scan(&link.ParadigmID, &link.EventGraphID, &link.SubgraphID); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListParadigmEvidenceLinksByUser(ctx context.Context, userID string) ([]ParadigmEvidenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.paradigm_id, l.event_graph_id, l.subgraph_id
		FROM paradigm_evidence_links l
		INNER JOIN paradigms p ON p.paradigm_id = l.paradigm_id
		WHERE p.user_id = ?
		ORDER BY l.paradigm_id ASC, l.event_graph_id ASC, l.subgraph_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ParadigmEvidenceLink, 0)
	for rows.Next() {
		var link ParadigmEvidenceLink
		if err := rows.Scan(&link.ParadigmID, &link.EventGraphID, &link.SubgraphID); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}
