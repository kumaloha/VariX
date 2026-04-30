package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

type EventInputCandidate struct {
	Scope             string   `json:"scope"`
	AnchorSubject     string   `json:"anchor_subject"`
	TimeBucket        string   `json:"time_bucket"`
	TimeStart         string   `json:"time_start,omitempty"`
	TimeEnd           string   `json:"time_end,omitempty"`
	SourceSubgraphIDs []string `json:"source_subgraph_ids,omitempty"`
	SourceArticleIDs  []string `json:"source_article_ids,omitempty"`
	NodeIDs           []string `json:"node_ids,omitempty"`
}

func (s *SQLiteStore) PersistMemoryContentGraph(ctx context.Context, userID string, subgraph graphmodel.ContentSubgraph, acceptedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := persistMemoryContentGraphSubgraphTx(ctx, tx, userID, subgraph, acceptedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.refreshProjectionLayersForUser(ctx, userID, acceptedAt)
}

func (s *SQLiteStore) PersistMemoryContentGraphDeferred(ctx context.Context, userID string, subgraph graphmodel.ContentSubgraph, acceptedAt time.Time) error {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := persistMemoryContentGraphSubgraphTx(ctx, tx, userID, subgraph, acceptedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.markContentGraphProjectionDirty(ctx, userID, subgraph, acceptedAt)
}

func persistMemoryContentGraphSubgraphTx(ctx context.Context, tx *sql.Tx, userID string, subgraph graphmodel.ContentSubgraph, acceptedAt time.Time) error {
	if err := subgraph.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(subgraph)
	if err != nil {
		return err
	}
	acceptedAt = normalizeRecordedTime(acceptedAt)
	_, err = tx.ExecContext(ctx, `INSERT INTO memory_content_graphs(user_id, source_platform, source_external_id, root_external_id, subgraph_id, payload_json, accepted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, source_platform, source_external_id) DO UPDATE SET
		  root_external_id = excluded.root_external_id,
		  subgraph_id = excluded.subgraph_id,
		  payload_json = excluded.payload_json,
		  accepted_at = excluded.accepted_at,
		  updated_at = excluded.updated_at`,
		userID,
		subgraph.SourcePlatform,
		subgraph.SourceExternalID,
		subgraph.RootExternalID,
		subgraph.ID,
		string(payload),
		acceptedAt.UTC().Format(time.RFC3339Nano),
		acceptedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("persist memory content graph: %w", err)
	}
	return replaceMemoryContentGraphSubjectsTx(ctx, tx, userID, subgraph, acceptedAt)
}

func (s *SQLiteStore) markContentGraphProjectionDirty(ctx context.Context, userID string, subgraph graphmodel.ContentSubgraph, at time.Time) error {
	return markContentGraphProjectionDirty(ctx, s.db, userID, subgraph, at)
}

func markContentGraphProjectionDirty(ctx context.Context, execer projectionDirtyExecer, userID string, subgraph graphmodel.ContentSubgraph, at time.Time) error {
	sourceRef := strings.TrimSpace(subgraph.SourcePlatform + ":" + subgraph.SourceExternalID)
	baseMarks := []ProjectionDirtyMark{
		{UserID: userID, Layer: "event", Reason: "content_graph_changed", SourceRef: sourceRef},
		{UserID: userID, Layer: "paradigm", Reason: "content_graph_changed", SourceRef: sourceRef},
		{UserID: userID, Layer: "global-v2", Reason: "content_graph_changed", SourceRef: sourceRef},
	}
	for _, mark := range baseMarks {
		if err := markProjectionDirty(ctx, execer, mark, at); err != nil {
			return err
		}
	}
	subjects := map[string]struct{}{}
	for _, node := range subgraph.Nodes {
		subject := strings.TrimSpace(node.SubjectCanonical)
		if subject == "" {
			subject = strings.TrimSpace(node.SubjectText)
		}
		if subject == "" {
			continue
		}
		subjects[subject] = struct{}{}
	}
	for subject := range subjects {
		for _, horizon := range []string{"1w", "1m", "1q", "1y", "2y", "5y"} {
			if err := markProjectionDirty(ctx, execer, ProjectionDirtyMark{UserID: userID, Layer: "subject-horizon", Subject: subject, Horizon: horizon, Reason: "content_graph_changed", SourceRef: sourceRef}, at); err != nil {
				return err
			}
		}
		if err := markProjectionDirty(ctx, execer, ProjectionDirtyMark{UserID: userID, Layer: "subject-experience", Subject: subject, Reason: "content_graph_changed", SourceRef: sourceRef}, at); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) BuildEventInputCandidates(ctx context.Context, userID string) ([]EventInputCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]*EventInputCandidate{}
	canonicalCache := map[string]string{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var subgraph graphmodel.ContentSubgraph
		if err := json.Unmarshal([]byte(payload), &subgraph); err != nil {
			return nil, fmt.Errorf("decode memory_content_graph payload: %w", err)
		}
		for _, node := range subgraph.Nodes {
			if !node.IsPrimary {
				continue
			}
			scope := eventCandidateScope(node)
			if scope == "" {
				continue
			}
			anchor, err := s.resolveCanonicalGraphNodeSubject(ctx, node, canonicalCache)
			if err != nil {
				return nil, err
			}
			if anchor == "" {
				continue
			}
			bucket := normalizedEventBucket(node.TimeBucket, deriveEventBucket(node))
			key := scope + "|" + anchor + "|" + bucket
			candidate := byKey[key]
			if candidate == nil {
				candidate = &EventInputCandidate{Scope: scope, AnchorSubject: anchor, TimeBucket: bucket}
				byKey[key] = candidate
			}
			candidate.SourceSubgraphIDs = append(candidate.SourceSubgraphIDs, subgraph.ID)
			candidate.SourceArticleIDs = append(candidate.SourceArticleIDs, subgraph.ArticleID)
			candidate.NodeIDs = append(candidate.NodeIDs, subgraph.ID+"::"+node.ID)
			mergeEventCandidateTimeWindow(candidate, node)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]EventInputCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		candidate.SourceSubgraphIDs = uniqueStrings(candidate.SourceSubgraphIDs)
		candidate.SourceArticleIDs = uniqueStrings(candidate.SourceArticleIDs)
		candidate.NodeIDs = uniqueStrings(candidate.NodeIDs)
		out = append(out, *candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AnchorSubject != out[j].AnchorSubject {
			return out[i].AnchorSubject < out[j].AnchorSubject
		}
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].TimeBucket < out[j].TimeBucket
	})
	return out, nil
}

func mergeEventCandidateTimeWindow(candidate *EventInputCandidate, node graphmodel.GraphNode) {
	start := firstEventNodeTime(node.TimeStart, node.TimeEnd, node.VerificationAsOf, node.LastVerifiedAt)
	end := firstEventNodeTime(node.TimeEnd, node.TimeStart, node.VerificationAsOf, node.LastVerifiedAt)
	if start != "" && (candidate.TimeStart == "" || eventTimeBefore(start, candidate.TimeStart)) {
		candidate.TimeStart = start
	}
	if end != "" && (candidate.TimeEnd == "" || eventTimeBefore(candidate.TimeEnd, end)) {
		candidate.TimeEnd = end
	}
}

func firstEventNodeTime(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			continue
		}
		return parsed.UTC().Format(time.RFC3339)
	}
	return ""
}

func eventTimeBefore(left, right string) bool {
	leftTime, leftErr := time.Parse(time.RFC3339, strings.TrimSpace(left))
	rightTime, rightErr := time.Parse(time.RFC3339, strings.TrimSpace(right))
	if leftErr == nil && rightErr == nil {
		return leftTime.Before(rightTime)
	}
	return strings.TrimSpace(left) < strings.TrimSpace(right)
}

func (s *SQLiteStore) refreshProjectionLayersForUser(ctx context.Context, userID string, at time.Time) error {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return err
	}
	at = normalizeNow(at)
	if _, err := s.RunEventGraphProjection(ctx, userID, at); err != nil {
		return err
	}
	if _, err := s.RunParadigmProjection(ctx, userID, at); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) resolveCanonicalSubject(ctx context.Context, subject string, cache map[string]string) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", nil
	}
	if cache != nil {
		if resolved, ok := cache[subject]; ok {
			return resolved, nil
		}
	}
	resolved := subject
	entity, err := s.FindCanonicalEntityByAlias(ctx, subject)
	switch {
	case err == nil:
		if display := normalizeCanonicalDisplay(entity.CanonicalName); display != "" {
			resolved = display
		}
	case errors.Is(err, sql.ErrNoRows):
		// keep original subject
	default:
		return "", err
	}
	if cache != nil {
		cache[subject] = resolved
	}
	return resolved, nil
}

func eventCandidateScope(node graphmodel.GraphNode) string {
	switch node.GraphRole {
	case graphmodel.GraphRoleDriver:
		return "driver"
	case graphmodel.GraphRoleTarget:
		return "target"
	default:
		return ""
	}
}

func deriveEventBucket(node graphmodel.GraphNode) string {
	timeBucket := strings.TrimSpace(node.TimeBucket)
	if timeBucket != "" {
		return timeBucket
	}
	timeStart := strings.TrimSpace(node.TimeStart)
	timeEnd := strings.TrimSpace(node.TimeEnd)
	if timeStart != "" && timeEnd != "" {
		start, errStart := time.Parse(time.RFC3339, timeStart)
		end, errEnd := time.Parse(time.RFC3339, timeEnd)
		if errStart == nil && errEnd == nil {
			days := int(end.Sub(start).Hours() / 24)
			switch {
			case days <= 1:
				return "1d"
			case days <= 7:
				return "1w"
			case days <= 31:
				return "1m"
			default:
				return "custom"
			}
		}
	}
	return ""
}
