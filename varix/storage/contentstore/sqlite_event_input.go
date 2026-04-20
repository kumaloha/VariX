package contentstore

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if _, err := s.RunEventGraphProjection(ctx, userID, acceptedAt); err != nil {
		return err
	}
	if _, err := s.RunParadigmProjection(ctx, userID, acceptedAt); err != nil {
		return err
	}
	return nil
}

func persistMemoryContentGraphSubgraphTx(ctx context.Context, tx *sql.Tx, userID string, subgraph graphmodel.ContentSubgraph, acceptedAt time.Time) error {
	if err := subgraph.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(subgraph)
	if err != nil {
		return err
	}
	if acceptedAt.IsZero() {
		acceptedAt = time.Now().UTC()
	}
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
	return nil
}

func (s *SQLiteStore) BuildEventInputCandidates(ctx context.Context, userID string) ([]EventInputCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json FROM memory_content_graphs WHERE user_id = ? ORDER BY source_platform ASC, source_external_id ASC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]*EventInputCandidate{}
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
			anchor := strings.TrimSpace(firstNonEmpty(node.SubjectCanonical, node.SubjectText))
			if anchor == "" {
				continue
			}
			bucket := strings.TrimSpace(firstNonEmpty(node.TimeBucket, deriveEventBucket(node)))
			if bucket == "" {
				bucket = "timeless"
			}
			key := scope + "|" + anchor + "|" + bucket
			candidate := byKey[key]
			if candidate == nil {
				candidate = &EventInputCandidate{Scope: scope, AnchorSubject: anchor, TimeBucket: bucket}
				byKey[key] = candidate
			}
			candidate.SourceSubgraphIDs = append(candidate.SourceSubgraphIDs, subgraph.ID)
			candidate.SourceArticleIDs = append(candidate.SourceArticleIDs, subgraph.ArticleID)
			candidate.NodeIDs = append(candidate.NodeIDs, subgraph.ID+"::"+node.ID)
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
	if strings.TrimSpace(node.TimeBucket) != "" {
		return strings.TrimSpace(node.TimeBucket)
	}
	if strings.TrimSpace(node.TimeStart) != "" && strings.TrimSpace(node.TimeEnd) != "" {
		start, errStart := time.Parse(time.RFC3339, node.TimeStart)
		end, errEnd := time.Parse(time.RFC3339, node.TimeEnd)
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
