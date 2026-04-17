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

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

var ErrMemoryOrganizationOutputStale = errors.New("memory organization output is stale")

type posteriorStateRow struct {
	State            memory.PosteriorState
	Diagnosis        memory.PosteriorDiagnosisCode
	Reason           string
	BlockedByNodeIDs []string
	UpdatedAt        *time.Time
}

func (s *SQLiteStore) RunNextMemoryOrganizationJob(ctx context.Context, userID string, now time.Time) (memory.OrganizationOutput, error) {
	var job memory.OrganizationJob
	var createdAt string
	query := `SELECT job_id, trigger_event_id, user_id, source_platform, source_external_id, status, created_at, started_at, finished_at
		FROM memory_organization_jobs
		WHERE status = 'queued'`
	args := []any{}
	if strings.TrimSpace(userID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(userID))
	}
	query += ` ORDER BY created_at ASC, job_id ASC LIMIT 1`
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&job.JobID, &job.TriggerEventID, &job.UserID, &job.SourcePlatform, &job.SourceExternalID, &job.Status, &createdAt, new(sql.NullString), new(sql.NullString))
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	job.CreatedAt = parseSQLiteTime(createdAt)
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_jobs SET status = ?, started_at = ? WHERE job_id = ?`, "running", now.Format(time.RFC3339Nano), job.JobID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	nodes, err := listUserMemoryBySourceTx(ctx, tx, job.UserID, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	posteriorByMemoryID, err := loadPosteriorStatesBySourceTx(ctx, tx, job.UserID, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	record, err := getCompiledOutputTx(ctx, tx, job.SourcePlatform, job.SourceExternalID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	factStatusByNode := factStatusMap(record)
	explicitConditionStatusByNode := explicitConditionStatusMap(record)
	predictionStatusByNode := predictionStatusMap(record)
	graphNodesByID := map[string]compile.GraphNode{}
	for _, node := range record.Output.Graph.Nodes {
		graphNodesByID[node.ID] = node
	}

	derivedNodes := make([]memory.AcceptedNode, 0, len(nodes))
	active := make([]memory.AcceptedNode, 0)
	inactive := make([]memory.AcceptedNode, 0)
	for _, node := range nodes {
		if posterior, ok := posteriorByMemoryID[node.MemoryID]; ok {
			node.PosteriorState = posterior.State
			node.PosteriorDiagnosis = posterior.Diagnosis
			node.PosteriorReason = posterior.Reason
			node.BlockedByNodeIDs = append([]string(nil), posterior.BlockedByNodeIDs...)
			node.PosteriorUpdatedAt = posterior.UpdatedAt
		}
		if derived, ok := graphNodesByID[node.NodeID]; ok {
			if sameNodeMeaning(node.NodeText, derived.Text) {
				if strings.TrimSpace(derived.Text) != "" {
					node.NodeText = derived.Text
				}
				if strings.TrimSpace(string(derived.Kind)) != "" {
					node.NodeKind = string(derived.Kind)
				}
				derivedStart, derivedEnd := derived.LegacyValidityWindow()
				if node.ValidFrom.IsZero() {
					node.ValidFrom = derivedStart
				}
				if node.ValidTo.IsZero() {
					node.ValidTo = derivedEnd
				}
			}
		}
		derivedNodes = append(derivedNodes, node)
		if isAcceptedNodeActiveAt(node, now) {
			active = append(active, node)
		} else {
			inactive = append(inactive, node)
		}
	}
	dedupeGroups := buildDedupeGroups(active, factStatusByNode, predictionStatusByNode)
	contradictionGroups := buildContradictionGroups(active)
	hierarchy := buildHierarchy(active, record)
	nodeHints := buildNodeHints(derivedNodes, active, dedupeGroups, contradictionGroups, hierarchy, factStatusByNode, explicitConditionStatusByNode, predictionStatusByNode)
	dominantDriver := buildDominantDriverSummary(active, nodeHints)
	nodeHints = applyDominantDriverRoles(nodeHints, dominantDriver)
	feedback := buildOrganizationFeedback(derivedNodes, nodeHints)

	output := memory.OrganizationOutput{
		JobID:               job.JobID,
		UserID:              job.UserID,
		SourcePlatform:      job.SourcePlatform,
		SourceExternalID:    job.SourceExternalID,
		GeneratedAt:         now,
		ActiveNodes:         active,
		InactiveNodes:       inactive,
		DedupeGroups:        dedupeGroups,
		ContradictionGroups: contradictionGroups,
		Hierarchy:           hierarchy,
		PredictionStatuses:  extractPredictionStatuses(nodes, record),
		FactVerifications:   extractFactVerifications(active, record),
		OpenQuestions:       buildOpenQuestions(active, record),
		NodeHints:           nodeHints,
		DominantDriver:      dominantDriver,
		Feedback:            feedback,
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO memory_organization_outputs(job_id, user_id, source_platform, source_external_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET created_at = excluded.created_at`,
		job.JobID, job.UserID, job.SourcePlatform, job.SourceExternalID, "{}", now.Format(time.RFC3339Nano))
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	outputID, _ := res.LastInsertId()
	if outputID == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT output_id FROM memory_organization_outputs WHERE job_id = ?`, job.JobID).Scan(&outputID)
	}
	output.OutputID = outputID
	payload, err := json.Marshal(output)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_outputs SET payload_json = ?, created_at = ? WHERE output_id = ?`, string(payload), now.Format(time.RFC3339Nano), outputID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE memory_organization_jobs SET status = ?, finished_at = ? WHERE job_id = ?`, "done", now.Format(time.RFC3339Nano), job.JobID); err != nil {
		return memory.OrganizationOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return memory.OrganizationOutput{}, err
	}
	return output, nil
}

func (s *SQLiteStore) GetLatestMemoryOrganizationOutput(ctx context.Context, userID, sourcePlatform, sourceExternalID string) (memory.OrganizationOutput, error) {
	var payload string
	var latestOutputCreatedAt string
	var latestOutputJobID int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload_json, created_at, job_id FROM memory_organization_outputs
		 WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		 ORDER BY created_at DESC, output_id DESC
		 LIMIT 1`,
		userID, sourcePlatform, sourceExternalID,
	).Scan(&payload, &latestOutputCreatedAt, &latestOutputJobID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	stale, staleJobID, staleJobStatus, staleJobCreatedAt, err := s.hasNewerInFlightOrganizationJob(ctx, userID, sourcePlatform, sourceExternalID, parseSQLiteTime(latestOutputCreatedAt), latestOutputJobID)
	if err != nil {
		return memory.OrganizationOutput{}, err
	}
	if stale {
		return memory.OrganizationOutput{}, fmt.Errorf("%w: source %s/%s for user %s has newer %s job %d created at %s", ErrMemoryOrganizationOutputStale, sourcePlatform, sourceExternalID, userID, staleJobStatus, staleJobID, staleJobCreatedAt.Format(time.RFC3339Nano))
	}
	var out memory.OrganizationOutput
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.OrganizationOutput{}, err
	}
	return out, nil
}

func (s *SQLiteStore) hasNewerInFlightOrganizationJob(ctx context.Context, userID, sourcePlatform, sourceExternalID string, latestOutputCreatedAt time.Time, latestOutputJobID int64) (bool, int64, string, time.Time, error) {
	var jobID int64
	var status string
	var createdAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT j.job_id, j.status, j.created_at
		 FROM memory_organization_jobs j
		 INNER JOIN memory_acceptance_events e ON e.event_id = j.trigger_event_id
		 WHERE j.user_id = ? AND j.source_platform = ? AND j.source_external_id = ? AND j.status IN ('queued', 'running') AND e.trigger_type = 'posterior_refresh'
		 ORDER BY created_at DESC, job_id DESC
		 LIMIT 1`,
		userID, sourcePlatform, sourceExternalID,
	).Scan(&jobID, &status, &createdAt)
	if err == sql.ErrNoRows {
		return false, 0, "", time.Time{}, nil
	}
	if err != nil {
		return false, 0, "", time.Time{}, err
	}
	jobCreatedAt := parseSQLiteTime(createdAt)
	if jobCreatedAt.After(latestOutputCreatedAt) || (jobCreatedAt.Equal(latestOutputCreatedAt) && jobID > latestOutputJobID) {
		return true, jobID, status, jobCreatedAt, nil
	}
	return false, 0, "", time.Time{}, nil
}

func listUserMemoryBySourceTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID string) ([]memory.AcceptedNode, error) {
	rows, err := tx.QueryContext(ctx, `SELECT memory_id, user_id, source_platform, source_external_id, root_external_id, node_id, node_kind, node_text, source_model, source_compiled_at, valid_from, valid_to, accepted_at
		FROM user_memory_nodes
		WHERE user_id = ? AND source_platform = ? AND source_external_id = ?
		ORDER BY accepted_at ASC, memory_id ASC`, userID, sourcePlatform, sourceExternalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryNodes(rows)
}

func getCompiledOutputTx(ctx context.Context, tx *sql.Tx, platform, externalID string) (compile.Record, error) {
	var payload string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM compiled_outputs WHERE platform = ? AND external_id = ?`, platform, externalID).Scan(&payload); err != nil {
		return compile.Record{}, err
	}
	var record compile.Record
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return compile.Record{}, err
	}
	return record, nil
}

func loadPosteriorStatesBySourceTx(ctx context.Context, tx *sql.Tx, userID, sourcePlatform, sourceExternalID string) (map[int64]posteriorStateRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT u.memory_id, p.state, p.diagnosis_code, p.reason, p.blocked_by_node_ids_json, p.updated_at
		FROM user_memory_nodes u
		INNER JOIN memory_posterior_states p
		  ON p.source_platform = u.source_platform
		 AND p.source_external_id = u.source_external_id
		 AND p.node_id = u.node_id
		WHERE u.user_id = ? AND u.source_platform = ? AND u.source_external_id = ?`,
		userID, sourcePlatform, sourceExternalID,
	)
	if err != nil {
		if isMissingPosteriorStateTableErr(err) {
			return map[int64]posteriorStateRow{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]posteriorStateRow)
	for rows.Next() {
		var memoryID int64
		var state sql.NullString
		var diagnosis sql.NullString
		var reason sql.NullString
		var blockedByNodeIDsJSON sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(&memoryID, &state, &diagnosis, &reason, &blockedByNodeIDsJSON, &updatedAt); err != nil {
			if isMissingPosteriorStateTableErr(err) {
				return map[int64]posteriorStateRow{}, nil
			}
			return nil, err
		}
		row := posteriorStateRow{
			State:     memory.PosteriorState(strings.TrimSpace(state.String)),
			Diagnosis: memory.PosteriorDiagnosisCode(strings.TrimSpace(diagnosis.String)),
			Reason:    strings.TrimSpace(reason.String),
		}
		if strings.TrimSpace(blockedByNodeIDsJSON.String) != "" {
			if err := json.Unmarshal([]byte(blockedByNodeIDsJSON.String), &row.BlockedByNodeIDs); err != nil {
				return nil, fmt.Errorf("decode posterior blocked_by_node_ids_json for memory_id %d: %w", memoryID, err)
			}
			sort.Strings(row.BlockedByNodeIDs)
		}
		if updatedAt.Valid && strings.TrimSpace(updatedAt.String) != "" {
			parsed := parseSQLiteTime(updatedAt.String)
			row.UpdatedAt = &parsed
		}
		out[memoryID] = row
	}
	if err := rows.Err(); err != nil {
		if isMissingPosteriorStateTableErr(err) {
			return map[int64]posteriorStateRow{}, nil
		}
		return nil, err
	}
	return out, nil
}

func isMissingPosteriorStateTableErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table: memory_posterior_states")
}

type canonicalNodeGroup struct {
	canonical string
	ids       []string
}

func buildDedupeGroups(nodes []memory.AcceptedNode, _ map[string]compile.FactStatus, _ map[string]compile.PredictionStatus) []memory.DedupeGroup {
	groups := groupNodesByCanonicalText(nodes)
	out := make([]memory.DedupeGroup, 0, len(groups))
	for _, group := range groups {
		if len(group.ids) <= 1 {
			continue
		}
		ids := append([]string(nil), group.ids...)
		out = append(out, memory.DedupeGroup{
			NodeIDs:              ids,
			RepresentativeNodeID: ids[0],
			CanonicalText:        group.canonical,
			Reason:               "canonicalized text match",
			Hint:                 "merge-near-duplicate",
		})
	}
	return out
}

func buildContradictionGroups(nodes []memory.AcceptedNode) []memory.ContradictionGroup {
	groups := groupNodesByCanonicalText(nodes)
	out := make([]memory.ContradictionGroup, 0)
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			reason, ok := contradictionReason(groups[i].canonical, groups[j].canonical)
			if !ok {
				continue
			}
			ids := append(append([]string(nil), groups[i].ids...), groups[j].ids...)
			sort.Strings(ids)
			out = append(out, memory.ContradictionGroup{
				NodeIDs: ids,
				Reason:  reason,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return joinNodeIDs(out[i].NodeIDs) < joinNodeIDs(out[j].NodeIDs)
	})
	return out
}

func groupNodesByCanonicalText(nodes []memory.AcceptedNode) []canonicalNodeGroup {
	byText := map[string][]string{}
	for _, node := range nodes {
		key := canonicalNodeText(node.NodeText)
		byText[key] = append(byText[key], node.NodeID)
	}
	keys := make([]string, 0, len(byText))
	for key := range byText {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]canonicalNodeGroup, 0, len(keys))
	for _, key := range keys {
		ids := append([]string(nil), byText[key]...)
		sort.Strings(ids)
		out = append(out, canonicalNodeGroup{
			canonical: key,
			ids:       ids,
		})
	}
	return out
}

func buildHierarchy(nodes []memory.AcceptedNode, record compile.Record) []memory.HierarchyLink {
	active := map[string]struct{}{}
	nodeKindByID := map[string]string{}
	for _, node := range nodes {
		active[node.NodeID] = struct{}{}
		nodeKindByID[node.NodeID] = node.NodeKind
	}
	factStatusByNode := map[string]compile.FactStatus{}
	for _, check := range record.Output.Verification.FactChecks {
		factStatusByNode[check.NodeID] = check.Status
	}
	out := make([]memory.HierarchyLink, 0)
	seen := map[string]struct{}{}
	for _, edge := range record.Output.Graph.Edges {
		if _, ok := active[edge.From]; !ok {
			continue
		}
		if _, ok := active[edge.To]; !ok {
			continue
		}
		if !hierarchyTransitionAllowed(nodeKindByID[edge.From], nodeKindByID[edge.To]) {
			continue
		}
		if status, ok := factStatusByNode[edge.From]; ok && status != compile.FactStatusClearlyTrue {
			continue
		}
		link := memory.HierarchyLink{
			ParentNodeID: edge.From,
			ParentKind:   nodeKindByID[edge.From],
			ChildNodeID:  edge.To,
			ChildKind:    nodeKindByID[edge.To],
			Kind:         string(edge.Kind),
			Source:       "graph",
			Hint:         graphHierarchyHint(edge.Kind),
		}
		key := link.ParentNodeID + "->" + link.ChildNodeID
		seen[key] = struct{}{}
		out = append(out, link)
	}

	nodesByKind := groupNodesByKind(nodes)
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeFact)], nodesByKind[string(compile.NodeExplicitCondition)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeFact)], nodesByKind[string(compile.NodeImplicitCondition)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeImplicitCondition)], nodesByKind[string(compile.NodeConclusion)])
	if len(nodesByKind[string(compile.NodeImplicitCondition)]) == 0 {
		addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeFact)], nodesByKind[string(compile.NodeConclusion)])
	}
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeExplicitCondition)], nodesByKind[string(compile.NodePrediction)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeConclusion)], nodesByKind[string(compile.NodePrediction)])
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentNodeID != out[j].ParentNodeID {
			return out[i].ParentNodeID < out[j].ParentNodeID
		}
		if out[i].ChildNodeID != out[j].ChildNodeID {
			return out[i].ChildNodeID < out[j].ChildNodeID
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Hint < out[j].Hint
	})
	return out
}

func groupNodesByKind(nodes []memory.AcceptedNode) map[string][]memory.AcceptedNode {
	out := map[string][]memory.AcceptedNode{}
	for _, node := range nodes {
		out[node.NodeKind] = append(out[node.NodeKind], node)
	}
	return out
}

func addInferredHierarchyLinks(out *[]memory.HierarchyLink, seen map[string]struct{}, factStatusByNode map[string]compile.FactStatus, parents, children []memory.AcceptedNode) {
	for _, parent := range parents {
		if status, ok := factStatusByNode[parent.NodeID]; ok && status != compile.FactStatusClearlyTrue {
			continue
		}
		for _, child := range children {
			if !hierarchyTransitionAllowed(parent.NodeKind, child.NodeKind) {
				continue
			}
			key := parent.NodeID + "->" + child.NodeID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			*out = append(*out, memory.HierarchyLink{
				ParentNodeID: parent.NodeID,
				ParentKind:   parent.NodeKind,
				ChildNodeID:  child.NodeID,
				ChildKind:    child.NodeKind,
				Kind:         "inferred",
				Source:       "inferred",
				Hint:         inferredHierarchyHint(parent.NodeKind, child.NodeKind),
			})
		}
	}
}

func hierarchyTransitionAllowed(parentKind, childKind string) bool {
	switch {
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeExplicitCondition):
		return true
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeImplicitCondition):
		return true
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodeImplicitCondition):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeImplicitCondition) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodePrediction):
		return true
	case parentKind == string(compile.NodeConclusion) && childKind == string(compile.NodePrediction):
		return true
	default:
		return false
	}
}

func extractPredictionStatuses(nodes []memory.AcceptedNode, record compile.Record) []memory.PredictionStatus {
	accepted := map[string]struct{}{}
	for _, node := range nodes {
		accepted[node.NodeID] = struct{}{}
	}
	out := make([]memory.PredictionStatus, 0)
	for _, check := range record.Output.Verification.PredictionChecks {
		if _, ok := accepted[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.PredictionStatus{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	return out
}

func extractFactVerifications(nodes []memory.AcceptedNode, record compile.Record) []memory.FactVerification {
	active := map[string]struct{}{}
	for _, node := range nodes {
		active[node.NodeID] = struct{}{}
	}
	out := make([]memory.FactVerification, 0)
	for _, check := range record.Output.Verification.FactChecks {
		if _, ok := active[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.FactVerification{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	for _, check := range record.Output.Verification.ImplicitConditionChecks {
		if _, ok := active[check.NodeID]; !ok {
			continue
		}
		out = append(out, memory.FactVerification{
			NodeID: check.NodeID,
			Status: string(check.Status),
			Reason: check.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func buildOpenQuestions(nodes []memory.AcceptedNode, record compile.Record) []string {
	questions := make([]string, 0)
	for _, node := range nodes {
		if node.ValidFrom.IsZero() && node.ValidTo.IsZero() && node.NodeKind != string(compile.NodeExplicitCondition) && node.NodeKind != string(compile.NodeConclusion) {
			questions = append(questions, fmt.Sprintf("node %s has no validity window", node.NodeID))
		}
		switch node.PosteriorState {
		case memory.PosteriorStatePending:
			questions = append(questions, fmt.Sprintf("posterior check pending for node %s", node.NodeID))
		case memory.PosteriorStateBlocked:
			if len(node.BlockedByNodeIDs) > 0 {
				questions = append(questions, fmt.Sprintf("node %s blocked by conditions: %s", node.NodeID, strings.Join(node.BlockedByNodeIDs, ", ")))
			} else {
				questions = append(questions, fmt.Sprintf("node %s remains posterior-blocked", node.NodeID))
			}
		case memory.PosteriorStateFalsified:
			if strings.TrimSpace(string(node.PosteriorDiagnosis)) != "" {
				questions = append(questions, fmt.Sprintf("node %s was falsified (%s)", node.NodeID, node.PosteriorDiagnosis))
			} else {
				questions = append(questions, fmt.Sprintf("node %s was falsified", node.NodeID))
			}
		}
	}
	for _, check := range record.Output.Verification.FactChecks {
		if check.Status == compile.FactStatusUnverifiable {
			questions = append(questions, fmt.Sprintf("fact node %s remains unverifiable", check.NodeID))
		}
	}
	for _, check := range record.Output.Verification.ImplicitConditionChecks {
		if check.Status == compile.FactStatusUnverifiable {
			questions = append(questions, fmt.Sprintf("implicit condition node %s remains unverifiable", check.NodeID))
		}
	}
	for _, check := range record.Output.Verification.ExplicitConditionChecks {
		if check.Status == compile.ExplicitConditionStatusUnknown {
			questions = append(questions, fmt.Sprintf("explicit condition node %s remains probability-unknown", check.NodeID))
		}
	}
	return questions
}

func isAcceptedNodeActiveAt(node memory.AcceptedNode, now time.Time) bool {
	switch node.NodeKind {
	case string(compile.NodeFact), string(compile.NodeImplicitCondition), string(compile.NodePrediction):
		if node.ValidFrom.IsZero() {
			return false
		}
		if !now.Before(node.ValidFrom) && (node.ValidTo.IsZero() || !now.After(node.ValidTo)) {
			return true
		}
		return false
	case string(compile.NodeExplicitCondition), string(compile.NodeConclusion):
		if node.ValidFrom.IsZero() && node.ValidTo.IsZero() {
			return true
		}
		if node.ValidFrom.IsZero() {
			return false
		}
		if node.ValidTo.IsZero() {
			return !now.Before(node.ValidFrom)
		}
		return !now.Before(node.ValidFrom) && !now.After(node.ValidTo)
	default:
		if node.ValidFrom.IsZero() {
			return false
		}
		if node.ValidTo.IsZero() {
			return !now.Before(node.ValidFrom)
		}
		return !now.Before(node.ValidFrom) && !now.After(node.ValidTo)
	}
}

func factStatusMap(record compile.Record) map[string]compile.FactStatus {
	out := make(map[string]compile.FactStatus, len(record.Output.Verification.FactChecks)+len(record.Output.Verification.ImplicitConditionChecks))
	for _, check := range record.Output.Verification.FactChecks {
		out[check.NodeID] = check.Status
	}
	for _, check := range record.Output.Verification.ImplicitConditionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func explicitConditionStatusMap(record compile.Record) map[string]compile.ExplicitConditionStatus {
	out := make(map[string]compile.ExplicitConditionStatus, len(record.Output.Verification.ExplicitConditionChecks))
	for _, check := range record.Output.Verification.ExplicitConditionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func predictionStatusMap(record compile.Record) map[string]compile.PredictionStatus {
	out := make(map[string]compile.PredictionStatus, len(record.Output.Verification.PredictionChecks))
	for _, check := range record.Output.Verification.PredictionChecks {
		out[check.NodeID] = check.Status
	}
	return out
}

func ensureNodeSet(m map[string]map[string]struct{}, key string) map[string]struct{} {
	if existing, ok := m[key]; ok {
		return existing
	}
	set := map[string]struct{}{}
	m[key] = set
	return set
}

func sortedNodeSet(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func buildNodeHints(nodes, active []memory.AcceptedNode, dedupeGroups []memory.DedupeGroup, contradictionGroups []memory.ContradictionGroup, hierarchy []memory.HierarchyLink, factStatusByNode map[string]compile.FactStatus, explicitConditionStatusByNode map[string]compile.ExplicitConditionStatus, predictionStatusByNode map[string]compile.PredictionStatus) []memory.NodeHint {
	activeSet := map[string]struct{}{}
	for _, node := range active {
		activeSet[node.NodeID] = struct{}{}
	}
	preferredSet := map[string]struct{}{}
	dedupePeers := map[string]map[string]struct{}{}
	for _, group := range dedupeGroups {
		if strings.TrimSpace(group.RepresentativeNodeID) != "" {
			preferredSet[group.RepresentativeNodeID] = struct{}{}
		}
		for _, nodeID := range group.NodeIDs {
			peerSet := ensureNodeSet(dedupePeers, nodeID)
			for _, peerID := range group.NodeIDs {
				if peerID == nodeID {
					continue
				}
				peerSet[peerID] = struct{}{}
			}
		}
	}
	contradictionPeers := map[string]map[string]struct{}{}
	for _, group := range contradictionGroups {
		for _, nodeID := range group.NodeIDs {
			peerSet := ensureNodeSet(contradictionPeers, nodeID)
			for _, peerID := range group.NodeIDs {
				if peerID == nodeID {
					continue
				}
				peerSet[peerID] = struct{}{}
			}
		}
	}
	parentIDs := map[string]map[string]struct{}{}
	childIDs := map[string]map[string]struct{}{}
	for _, link := range hierarchy {
		ensureNodeSet(parentIDs, link.ChildNodeID)[link.ParentNodeID] = struct{}{}
		ensureNodeSet(childIDs, link.ParentNodeID)[link.ChildNodeID] = struct{}{}
	}
	out := make([]memory.NodeHint, 0, len(nodes))
	for _, node := range nodes {
		hint := memory.NodeHint{NodeID: node.NodeID}
		if _, ok := activeSet[node.NodeID]; ok {
			hint.State = "active"
		} else {
			hint.State = "inactive"
		}
		if _, ok := preferredSet[node.NodeID]; ok {
			hint.PreferredForDisplay = true
		}
		if status, ok := factStatusByNode[node.NodeID]; ok {
			hint.VerificationStatus = string(status)
		}
		if status, ok := explicitConditionStatusByNode[node.NodeID]; ok {
			hint.ConditionProbability = string(status)
		}
		if status, ok := predictionStatusByNode[node.NodeID]; ok {
			hint.PredictionStatus = string(status)
		}
		hint.PosteriorState = node.PosteriorState
		hint.PosteriorDiagnosis = node.PosteriorDiagnosis
		hint.PosteriorReason = node.PosteriorReason
		hint.BlockedByNodeIDs = append([]string(nil), node.BlockedByNodeIDs...)
		hint.DedupePeerNodeIDs = sortedNodeSet(dedupePeers[node.NodeID])
		hint.ContradictionNodeIDs = sortedNodeSet(contradictionPeers[node.NodeID])
		hint.ParentNodeIDs = sortedNodeSet(parentIDs[node.NodeID])
		hint.ChildNodeIDs = sortedNodeSet(childIDs[node.NodeID])
		switch node.PosteriorState {
		case memory.PosteriorStateVerified:
			hint.PreferredForDisplay = true
		case memory.PosteriorStateBlocked, memory.PosteriorStateFalsified:
			hint.PreferredForDisplay = false
		}
		if hint.State == "active" {
			switch {
			case len(hint.ParentNodeIDs) == 0 && len(hint.ChildNodeIDs) > 0:
				hint.HierarchyRole = "root"
			case len(hint.ParentNodeIDs) > 0 && len(hint.ChildNodeIDs) == 0:
				hint.HierarchyRole = "leaf"
			case len(hint.ParentNodeIDs) > 0 && len(hint.ChildNodeIDs) > 0:
				hint.HierarchyRole = "bridge"
			default:
				hint.HierarchyRole = "isolated"
			}
		}
		hint.NodeVerdict = deriveNodeVerdict(node, hint)
		out = append(out, hint)
	}
	return out
}

func deriveNodeVerdict(node memory.AcceptedNode, hint memory.NodeHint) string {
	switch node.PosteriorState {
	case memory.PosteriorStateFalsified:
		return "falsified"
	case memory.PosteriorStateBlocked:
		return "blocked"
	case memory.PosteriorStateVerified:
		return "supported"
	}
	switch hint.VerificationStatus {
	case string(compile.FactStatusClearlyTrue):
		return "supported"
	case string(compile.FactStatusClearlyFalse):
		return "contradicted"
	case string(compile.FactStatusUnverifiable):
		return "needs_review"
	}
	switch hint.ConditionProbability {
	case string(compile.ExplicitConditionStatusHigh), string(compile.ExplicitConditionStatusMedium), string(compile.ExplicitConditionStatusLow):
		return "supported"
	case string(compile.ExplicitConditionStatusUnknown):
		return "needs_review"
	}
	switch hint.PredictionStatus {
	case string(compile.PredictionStatusResolvedTrue):
		return "supported"
	case string(compile.PredictionStatusResolvedFalse):
		return "contradicted"
	case string(compile.PredictionStatusUnresolved), string(compile.PredictionStatusStaleUnresolved):
		return "needs_review"
	}
	if node.PosteriorState == memory.PosteriorStatePending {
		return "needs_review"
	}
	if len(hint.ContradictionNodeIDs) > 0 {
		return "contested"
	}
	if hint.State == "inactive" {
		return "inactive"
	}
	return "active"
}

func buildDominantDriverSummary(active []memory.AcceptedNode, hints []memory.NodeHint) *memory.DominantDriverSummary {
	if len(active) == 0 || len(hints) == 0 {
		return nil
	}
	nodesByID := make(map[string]memory.AcceptedNode, len(active))
	hintsByID := make(map[string]memory.NodeHint, len(hints))
	childrenByID := map[string][]string{}
	for _, node := range active {
		nodesByID[node.NodeID] = node
	}
	for _, hint := range hints {
		if _, ok := nodesByID[hint.NodeID]; !ok {
			continue
		}
		hintsByID[hint.NodeID] = hint
		childrenByID[hint.NodeID] = append([]string(nil), hint.ChildNodeIDs...)
	}

	candidates := make([]memory.AcceptedNode, 0)
	for _, node := range active {
		hint, ok := hintsByID[node.NodeID]
		if !ok {
			continue
		}
		if isDriverKind(node.NodeKind) && (len(hint.ParentNodeIDs) == 0 || len(hint.ChildNodeIDs) > 0) {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		for _, node := range active {
			if node.NodeKind == string(compile.NodePrediction) {
				continue
			}
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	type scoredDriver struct {
		node       memory.AcceptedNode
		descendant int
		score      int
	}
	scored := make([]scoredDriver, 0, len(candidates))
	for _, node := range candidates {
		hint := hintsByID[node.NodeID]
		descendantCount := countDescendants(node.NodeID, childrenByID)
		score := descendantCount*10 + len(hint.ChildNodeIDs)*3 + driverKindWeight(node.NodeKind) + verdictWeight(hint.NodeVerdict)
		if hint.HierarchyRole == "root" {
			score += 5
		}
		if hint.PreferredForDisplay {
			score += 2
		}
		scored = append(scored, scoredDriver{
			node:       node,
			descendant: descendantCount,
			score:      score,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].descendant != scored[j].descendant {
			return scored[i].descendant > scored[j].descendant
		}
		return scored[i].node.NodeID < scored[j].node.NodeID
	})

	primary := scored[0]
	supporting := make([]string, 0, len(scored)-1)
	supportingTexts := make([]string, 0, len(scored)-1)
	for _, candidate := range scored[1:] {
		supporting = append(supporting, candidate.node.NodeID)
		supportingTexts = append(supportingTexts, firstNonBlank(candidate.node.NodeText, candidate.node.NodeID))
	}

	explanation := fmt.Sprintf("%s is the primary driver because it reaches %d downstream node(s) with the strongest current verdict.",
		firstNonBlank(primary.node.NodeText, primary.node.NodeID),
		primary.descendant,
	)
	if len(supporting) > 0 {
		explanation += fmt.Sprintf(" %s remain supporting because they carry weaker or narrower evidence in the same source path.",
			strings.Join(supportingTexts, ", "),
		)
	}

	return &memory.DominantDriverSummary{
		NodeID:            primary.node.NodeID,
		NodeKind:          primary.node.NodeKind,
		NodeText:          primary.node.NodeText,
		SupportingNodeIDs: supporting,
		Explanation:       explanation,
	}
}

func applyDominantDriverRoles(hints []memory.NodeHint, dominant *memory.DominantDriverSummary) []memory.NodeHint {
	if dominant == nil {
		return hints
	}
	supporting := map[string]struct{}{}
	for _, nodeID := range dominant.SupportingNodeIDs {
		supporting[nodeID] = struct{}{}
	}
	out := make([]memory.NodeHint, 0, len(hints))
	for _, hint := range hints {
		switch {
		case hint.NodeID == dominant.NodeID:
			hint.DriverRole = "primary"
			hint.PreferredForDisplay = true
		case len(supporting) > 0:
			if _, ok := supporting[hint.NodeID]; ok {
				hint.DriverRole = "supporting"
			}
		}
		out = append(out, hint)
	}
	return out
}

func buildOrganizationFeedback(nodes []memory.AcceptedNode, hints []memory.NodeHint) []memory.OrganizationFeedback {
	nodesByID := make(map[string]memory.AcceptedNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.NodeID] = node
	}
	type rankedFeedback struct {
		item     memory.OrganizationFeedback
		severity int
		priority int
	}
	ranked := make([]rankedFeedback, 0, len(hints))
	for _, hint := range hints {
		node, ok := nodesByID[hint.NodeID]
		if !ok {
			continue
		}
		feedback, severity, priority, ok := feedbackForHint(node, hint)
		if !ok {
			continue
		}
		ranked = append(ranked, rankedFeedback{
			item:     feedback,
			severity: severity,
			priority: priority,
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].severity != ranked[j].severity {
			return ranked[i].severity > ranked[j].severity
		}
		if ranked[i].priority != ranked[j].priority {
			return ranked[i].priority > ranked[j].priority
		}
		return ranked[i].item.NodeID < ranked[j].item.NodeID
	})
	out := make([]memory.OrganizationFeedback, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.item)
	}
	return out
}

func feedbackForHint(node memory.AcceptedNode, hint memory.NodeHint) (memory.OrganizationFeedback, int, int, bool) {
	base := memory.OrganizationFeedback{
		NodeID:   node.NodeID,
		NodeText: node.NodeText,
		NodeKind: node.NodeKind,
	}
	label := firstNonBlank(node.NodeText, node.NodeID)
	switch {
	case node.PosteriorState == memory.PosteriorStateFalsified:
		base.Severity = "error"
		base.Code = "posterior_falsified"
		base.Message = fmt.Sprintf("%s is falsified by posterior verification.", label)
		base.Reason = node.PosteriorReason
		return base, 3, 9, true
	case node.PosteriorState == memory.PosteriorStateBlocked:
		base.Severity = "error"
		base.Code = "posterior_blocked"
		base.Message = fmt.Sprintf("%s is blocked until its required conditions are resolved.", label)
		base.Reason = node.PosteriorReason
		return base, 3, 8, true
	case hint.VerificationStatus == string(compile.FactStatusClearlyFalse):
		base.Severity = "error"
		base.Code = "fact_contradicted"
		base.Message = fmt.Sprintf("%s is contradicted by fact verification.", label)
		return base, 3, 7, true
	case hint.PredictionStatus == string(compile.PredictionStatusResolvedFalse):
		base.Severity = "error"
		base.Code = "prediction_missed"
		base.Message = fmt.Sprintf("%s resolved false and should be treated as a failed prediction.", label)
		return base, 3, 6, true
	case len(hint.ContradictionNodeIDs) > 0:
		base.Severity = "warning"
		base.Code = "conflicting_nodes"
		base.Message = fmt.Sprintf("%s conflicts with node(s) %s.", label, strings.Join(hint.ContradictionNodeIDs, ", "))
		return base, 2, 5, true
	case node.PosteriorState == memory.PosteriorStatePending:
		base.Severity = "warning"
		base.Code = "posterior_pending"
		base.Message = fmt.Sprintf("%s still needs posterior verification.", label)
		base.Reason = node.PosteriorReason
		return base, 2, 4, true
	case hint.VerificationStatus == string(compile.FactStatusUnverifiable):
		base.Severity = "warning"
		base.Code = "needs_evidence"
		base.Message = fmt.Sprintf("%s remains unverifiable and needs stronger evidence.", label)
		return base, 2, 3, true
	case hint.ConditionProbability == string(compile.ExplicitConditionStatusUnknown):
		base.Severity = "warning"
		base.Code = "condition_unknown"
		base.Message = fmt.Sprintf("%s still has an unknown condition probability.", label)
		return base, 2, 2, true
	case hint.PredictionStatus == string(compile.PredictionStatusUnresolved) || hint.PredictionStatus == string(compile.PredictionStatusStaleUnresolved):
		base.Severity = "warning"
		base.Code = "prediction_open"
		base.Message = fmt.Sprintf("%s still needs prediction follow-through.", label)
		return base, 2, 1, true
	case len(hint.DedupePeerNodeIDs) > 0 && !hint.PreferredForDisplay:
		base.Severity = "info"
		base.Code = "near_duplicate"
		base.Message = fmt.Sprintf("%s overlaps with near-duplicate node(s) %s.", label, strings.Join(hint.DedupePeerNodeIDs, ", "))
		return base, 1, 1, true
	default:
		return memory.OrganizationFeedback{}, 0, 0, false
	}
}

func isDriverKind(kind string) bool {
	switch kind {
	case string(compile.NodeFact), string(compile.NodeExplicitCondition), string(compile.NodeImplicitCondition):
		return true
	default:
		return false
	}
}

func countDescendants(nodeID string, childrenByID map[string][]string) int {
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(current string) {
		for _, childID := range childrenByID[current] {
			if _, ok := seen[childID]; ok {
				continue
			}
			seen[childID] = struct{}{}
			walk(childID)
		}
	}
	walk(nodeID)
	return len(seen)
}

func driverKindWeight(kind string) int {
	switch kind {
	case string(compile.NodeFact):
		return 40
	case string(compile.NodeExplicitCondition):
		return 35
	case string(compile.NodeImplicitCondition):
		return 30
	case string(compile.NodeConclusion):
		return 20
	case string(compile.NodePrediction):
		return 10
	default:
		return 0
	}
}

func verdictWeight(verdict string) int {
	switch verdict {
	case "supported":
		return 20
	case "active":
		return 10
	case "needs_review":
		return 0
	case "contested":
		return -5
	case "blocked":
		return -20
	case "falsified", "contradicted":
		return -25
	case "inactive":
		return -30
	default:
		return 0
	}
}

func normalizeNodeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func canonicalNodeText(text string) string {
	normalized := normalizeNodeText(text)
	replacer := strings.NewReplacer(
		"，", "",
		"。", "",
		"！", "",
		"？", "",
		"!", "",
		"?", "",
		".", "",
		",", "",
		"：", "",
		"；", "",
		"、", "",
		"“", "",
		"”", "",
		"如果", "",
		"若", "",
		"一旦", "",
		"假如", "",
		"倘若", "",
		"如若", "",
		"发生", "",
		"（", "",
		"）", "",
		"(", "",
		")", "",
		"继续", "",
		"仍", "",
		"预计", "",
		"预期", "",
		"可能", "",
		"有望", "",
		"正在", "",
		"会", "",
		"将", "",
		"将会", "",
		"走高", "上升",
		"上涨", "上升",
		"攀升", "上升",
		"回升", "上升",
		"下滑", "下降",
		"下跌", "下降",
		"走低", "下降",
		"回落", "下降",
		"上行", "上升",
		"下行", "下降",
		"走弱", "下降",
		"走强", "上升",
		"减弱", "削弱",
		"强化", "增强",
		"支撑", "支持",
	)
	return replacer.Replace(normalized)
}

func sameNodeMeaning(a, b string) bool {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func areContradictory(a, b string) bool {
	_, ok := contradictionReason(a, b)
	return ok
}

func contradictionReason(a, b string) (string, bool) {
	a = canonicalNodeText(a)
	b = canonicalNodeText(b)
	if strings.ReplaceAll(a, "不", "") == b || strings.ReplaceAll(b, "不", "") == a {
		return "negation contradiction", true
	}
	if strings.ReplaceAll(a, "不会", "") == b || strings.ReplaceAll(b, "不会", "") == a {
		return "negation contradiction", true
	}
	for _, pair := range [][2]string{
		{"上升", "下降"},
		{"增加", "减少"},
		{"恶化", "改善"},
		{"紧张", "缓和"},
		{"收缩", "扩张"},
		{"宽松", "收紧"},
		{"削弱", "增强"},
		{"利多", "利空"},
		{"支持", "压制"},
		{"升温", "降温"},
	} {
		if strings.ReplaceAll(a, pair[0], pair[1]) == b || strings.ReplaceAll(a, pair[1], pair[0]) == b {
			return "antonym contradiction", true
		}
		if strings.ReplaceAll(b, pair[0], pair[1]) == a || strings.ReplaceAll(b, pair[1], pair[0]) == a {
			return "antonym contradiction", true
		}
	}
	return "", false
}

func graphHierarchyHint(kind compile.EdgeKind) string {
	switch kind {
	case compile.EdgeDerives:
		return "compiled-derives"
	case compile.EdgePositive:
		return "compiled-supports"
	case compile.EdgeNegative:
		return "compiled-challenges"
	case compile.EdgePresets:
		return "compiled-presets"
	default:
		return "compiled-link"
	}
}

func inferredHierarchyHint(parentKind, childKind string) string {
	return nodeKindSlug(parentKind) + "-to-" + nodeKindSlug(childKind)
}

func nodeKindSlug(kind string) string {
	switch kind {
	case string(compile.NodeFact):
		return "fact"
	case string(compile.NodeExplicitCondition):
		return "explicit-condition"
	case string(compile.NodeAssumption):
		return "implicit-condition"
	case string(compile.NodeConclusion):
		return "conclusion"
	case string(compile.NodePrediction):
		return "prediction"
	default:
		return "node"
	}
}

func joinNodeIDs(ids []string) string {
	return strings.Join(ids, "\x00")
}
