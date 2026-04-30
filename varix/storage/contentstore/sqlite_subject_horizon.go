package contentstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

type subjectHorizonSpec struct {
	Horizon       string
	RefreshPolicy string
	WindowStart   func(time.Time) time.Time
	NextRefresh   func(time.Time) time.Time
}

type projectionSubjectResolver func(context.Context, graphmodel.GraphNode) (string, error)

func (s *SQLiteStore) GetSubjectHorizonMemory(ctx context.Context, userID, subject, horizon string, now time.Time, refresh bool) (memory.SubjectHorizonMemory, error) {
	return s.getSubjectHorizonMemory(ctx, userID, subject, horizon, now, refresh, nil, false, nil)
}

func (s *SQLiteStore) getSubjectHorizonMemory(ctx context.Context, userID, subject, horizon string, now time.Time, refresh bool, graphInputs []graphmodel.ContentSubgraph, hasGraphInputs bool, resolveSubject projectionSubjectResolver) (memory.SubjectHorizonMemory, error) {
	userID, err := normalizeRequiredUserID(userID)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return memory.SubjectHorizonMemory{}, fmt.Errorf("subject is required")
	}
	spec, err := subjectHorizonSpecFor(horizon)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	now = normalizeNow(now)
	canonicalSubject, err := s.resolveCanonicalListSubject(ctx, subject)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	if !refresh {
		cached, ok, err := s.getCachedSubjectHorizonMemory(ctx, userID, canonicalSubject, spec.Horizon, now)
		if err != nil {
			return memory.SubjectHorizonMemory{}, err
		}
		if ok {
			return cached, nil
		}
	}
	if !hasGraphInputs {
		graphInputs, err = s.ListMemoryContentGraphsBySubject(ctx, userID, canonicalSubject)
		if err != nil {
			return memory.SubjectHorizonMemory{}, err
		}
	}
	out, err := s.buildSubjectHorizonMemoryFromGraphs(ctx, userID, subject, canonicalSubject, spec, now, graphInputs, map[string]string{}, resolveSubject)
	if err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	if err := s.upsertSubjectHorizonMemory(ctx, out); err != nil {
		return memory.SubjectHorizonMemory{}, err
	}
	return out, nil
}

func subjectHorizonSpecFor(horizon string) (subjectHorizonSpec, error) {
	switch strings.TrimSpace(horizon) {
	case "1w":
		return subjectHorizonSpec{"1w", "daily", func(t time.Time) time.Time { return t.AddDate(0, 0, -7) }, func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }}, nil
	case "1m":
		return subjectHorizonSpec{"1m", "weekly", func(t time.Time) time.Time { return t.AddDate(0, -1, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }}, nil
	case "1q":
		return subjectHorizonSpec{"1q", "monthly", func(t time.Time) time.Time { return t.AddDate(0, -3, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }}, nil
	case "1y":
		return subjectHorizonSpec{"1y", "quarterly", func(t time.Time) time.Time { return t.AddDate(-1, 0, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 3, 0) }}, nil
	case "2y":
		return subjectHorizonSpec{"2y", "semiannual", func(t time.Time) time.Time { return t.AddDate(-2, 0, 0) }, func(t time.Time) time.Time { return t.AddDate(0, 6, 0) }}, nil
	case "5y":
		return subjectHorizonSpec{"5y", "annual", func(t time.Time) time.Time { return t.AddDate(-5, 0, 0) }, func(t time.Time) time.Time { return t.AddDate(1, 0, 0) }}, nil
	default:
		return subjectHorizonSpec{}, fmt.Errorf("unsupported subject horizon %q; supported: 1w, 1m, 1q, 1y, 2y, 5y", horizon)
	}
}

func (s *SQLiteStore) getCachedSubjectHorizonMemory(ctx context.Context, userID, canonicalSubject, horizon string, now time.Time) (memory.SubjectHorizonMemory, bool, error) {
	var payload, nextRefreshAt string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json, next_refresh_at FROM subject_horizon_memories WHERE user_id = ? AND canonical_subject = ? AND horizon = ?`, userID, canonicalSubject, horizon).Scan(&payload, &nextRefreshAt)
	if err == sql.ErrNoRows {
		return memory.SubjectHorizonMemory{}, false, nil
	}
	if err != nil {
		return memory.SubjectHorizonMemory{}, false, err
	}
	next, err := time.Parse(time.RFC3339, strings.TrimSpace(nextRefreshAt))
	if err != nil {
		return memory.SubjectHorizonMemory{}, false, nil
	}
	if !now.Before(next) {
		return memory.SubjectHorizonMemory{}, false, nil
	}
	var out memory.SubjectHorizonMemory
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.SubjectHorizonMemory{}, false, fmt.Errorf("decode subject horizon memory: %w", err)
	}
	out.CacheStatus = "fresh"
	return out, true, nil
}

func (s *SQLiteStore) buildSubjectHorizonMemoryFromGraphs(ctx context.Context, userID, subject, canonicalSubject string, spec subjectHorizonSpec, now time.Time, graphs []graphmodel.ContentSubgraph, cache map[string]string, resolveSubject projectionSubjectResolver) (memory.SubjectHorizonMemory, error) {
	windowStart := spec.WindowStart(now).UTC()
	windowEnd := now.UTC()
	if cache == nil {
		cache = map[string]string{}
	}
	if resolveSubject == nil {
		resolveSubject = func(ctx context.Context, node graphmodel.GraphNode) (string, error) {
			return s.resolveCanonicalGraphNodeSubject(ctx, node, cache)
		}
	}
	keyChanges := make([]memory.SubjectHorizonChange, 0)
	driverClusters := map[string]*memory.SubjectHorizonDriver{}
	evidenceRefs := make([]string, 0)
	contradictions := make([]memory.SubjectHorizonConflict, 0)
	sourceSet := map[string]struct{}{}
	for _, graph := range graphs {
		graphDrivers := primaryGraphDrivers(graph)
		for _, node := range graph.Nodes {
			if !isSubjectTimelineNode(node) {
				continue
			}
			nodeSubject, err := resolveSubject(ctx, node)
			if err != nil {
				return memory.SubjectHorizonMemory{}, err
			}
			if !subjectMatchesTimelineQuery(node, nodeSubject, canonicalSubject) {
				continue
			}
			when, ok := subjectHorizonEntryTime(graph, node)
			if !ok || when.Before(windowStart) || when.After(windowEnd) {
				continue
			}
			entry := subjectTimelineEntry(graph, node)
			change := memory.SubjectHorizonChange{
				When:             when.UTC().Format(time.RFC3339),
				Subject:          firstTrimmed(entry.SubjectCanonical, entry.SubjectText),
				ChangeText:       entry.ChangeText,
				SourcePlatform:   entry.SourcePlatform,
				SourceExternalID: entry.SourceExternalID,
				NodeID:           entry.NodeID,
			}
			keyChanges = append(keyChanges, change)
			ref := entry.SourcePlatform + ":" + entry.SourceExternalID + "#" + entry.NodeID
			evidenceRefs = append(evidenceRefs, ref)
			sourceSet[entry.SourcePlatform+":"+entry.SourceExternalID] = struct{}{}
			addSubjectHorizonDrivers(driverClusters, graph, graphDrivers, node, entry.SourcePlatform+":"+entry.SourceExternalID)
		}
	}
	sort.SliceStable(keyChanges, func(i, j int) bool {
		if keyChanges[i].When != keyChanges[j].When {
			return keyChanges[i].When < keyChanges[j].When
		}
		return keyChanges[i].SourceExternalID < keyChanges[j].SourceExternalID
	})
	for i := range keyChanges {
		keyChanges[i].RelationToPrior = classifySubjectHorizonChangeRelation(keyChanges[:i], keyChanges[i])
		if keyChanges[i].RelationToPrior == memory.SubjectChangeContradicts && i > 0 {
			contradictions = append(contradictions, memory.SubjectHorizonConflict{PreviousChange: keyChanges[i-1].ChangeText, CurrentChange: keyChanges[i].ChangeText, At: keyChanges[i].When})
		}
	}
	drivers := subjectHorizonDriverList(driverClusters)
	generatedAt := now.UTC().Format(time.RFC3339)
	out := memory.SubjectHorizonMemory{
		UserID:             userID,
		Subject:            subject,
		CanonicalSubject:   canonicalSubject,
		Horizon:            spec.Horizon,
		RefreshPolicy:      spec.RefreshPolicy,
		WindowStart:        windowStart.Format(time.RFC3339),
		WindowEnd:          windowEnd.Format(time.RFC3339),
		GeneratedAt:        generatedAt,
		LastRefreshedAt:    generatedAt,
		NextRefreshAt:      spec.NextRefresh(now).UTC().Format(time.RFC3339),
		CacheStatus:        "refreshed",
		SampleCount:        len(keyChanges),
		SourceCount:        len(sourceSet),
		KeyChanges:         keyChanges,
		DriverClusters:     drivers,
		Contradictions:     contradictions,
		EvidenceSourceRefs: uniqueStrings(evidenceRefs),
	}
	out.TrendDirection, out.VolatilityState, out.DominantPattern = summarizeSubjectHorizonPattern(keyChanges)
	out.Abstraction = summarizeSubjectHorizonAbstraction(out)
	out.InputHash = subjectHorizonInputHash(out)
	return out, nil
}

func classifySubjectHorizonChangeRelation(prior []memory.SubjectHorizonChange, current memory.SubjectHorizonChange) memory.SubjectChangeRelation {
	entries := make([]memory.SubjectChangeEntry, 0, len(prior))
	for _, item := range prior {
		entries = append(entries, memory.SubjectChangeEntry{ChangeText: item.ChangeText, SubjectText: item.Subject})
	}
	return classifySubjectChangeRelation(entries, memory.SubjectChangeEntry{SubjectText: current.Subject, ChangeText: current.ChangeText})
}

func subjectHorizonEntryTime(graph graphmodel.ContentSubgraph, node graphmodel.GraphNode) (time.Time, bool) {
	for _, value := range []string{node.TimeStart, node.TimeEnd, node.VerificationAsOf, graph.CompiledAt, graph.UpdatedAt} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func primaryGraphDrivers(graph graphmodel.ContentSubgraph) []graphmodel.GraphNode {
	drivers := make([]graphmodel.GraphNode, 0)
	for _, node := range graph.Nodes {
		if node.IsPrimary && node.GraphRole == graphmodel.GraphRoleDriver {
			drivers = append(drivers, node)
		}
	}
	return drivers
}

func addSubjectHorizonDrivers(clusters map[string]*memory.SubjectHorizonDriver, graph graphmodel.ContentSubgraph, drivers []graphmodel.GraphNode, target graphmodel.GraphNode, sourceRef string) {
	for _, driver := range drivers {
		subject := firstTrimmed(driver.SubjectCanonical, driver.SubjectText)
		if subject == "" {
			continue
		}
		cluster := clusters[subject]
		if cluster == nil {
			cluster = &memory.SubjectHorizonDriver{Subject: subject}
			clusters[subject] = cluster
		}
		if change := strings.TrimSpace(driver.ChangeText); change != "" {
			cluster.Changes = uniqueStrings(append(cluster.Changes, change))
		}
		if path := subjectHorizonRelationPath(graph, driver.ID, target.ID); path != "" {
			cluster.RelationPaths = uniqueStrings(append(cluster.RelationPaths, path))
		}
		cluster.SourceRefs = uniqueStrings(append(cluster.SourceRefs, sourceRef+"#"+driver.ID))
		cluster.Count++
	}
}

func subjectHorizonRelationPath(graph graphmodel.ContentSubgraph, fromID, toID string) string {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" || fromID == toID {
		return ""
	}
	nodes := map[string]graphmodel.GraphNode{}
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}
	if _, ok := nodes[fromID]; !ok {
		return ""
	}
	if _, ok := nodes[toID]; !ok {
		return ""
	}
	adj := map[string][]string{}
	for _, edge := range graph.Edges {
		switch edge.Type {
		case graphmodel.EdgeTypeDrives, graphmodel.EdgeTypeExplains, graphmodel.EdgeTypeSupports:
		default:
			continue
		}
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	type queueItem struct {
		id   string
		path []string
	}
	queue := []queueItem{{id: fromID, path: []string{fromID}}}
	seen := map[string]struct{}{fromID: {}}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if len(item.path) > 6 {
			continue
		}
		for _, next := range adj[item.id] {
			if _, ok := seen[next]; ok {
				continue
			}
			nextPath := append(append([]string(nil), item.path...), next)
			if next == toID {
				return subjectHorizonPathLabel(nodes, nextPath)
			}
			seen[next] = struct{}{}
			queue = append(queue, queueItem{id: next, path: nextPath})
		}
	}
	return ""
}

func subjectHorizonPathLabel(nodes map[string]graphmodel.GraphNode, path []string) string {
	labels := make([]string, 0, len(path))
	for _, id := range path {
		node := nodes[id]
		label := firstTrimmed(node.SubjectCanonical, node.SubjectText, node.ChangeText)
		if label == "" {
			return ""
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, " -> ")
}

func subjectHorizonDriverList(clusters map[string]*memory.SubjectHorizonDriver) []memory.SubjectHorizonDriver {
	out := make([]memory.SubjectHorizonDriver, 0, len(clusters))
	for _, cluster := range clusters {
		out = append(out, *cluster)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Subject < out[j].Subject
	})
	return out
}

func summarizeSubjectHorizonPattern(changes []memory.SubjectHorizonChange) (trend, volatility, pattern string) {
	up, down := 0, 0
	for _, change := range changes {
		text := normalizeSubjectChangeText(change.ChangeText)
		if containsAny(text, "上涨", "创新高", "走强", "反弹", "上升") {
			up++
		}
		if containsAny(text, "下跌", "回落", "承压", "走弱", "下降") {
			down++
		}
	}
	switch {
	case up > 0 && down > 0:
		trend = "mixed"
	case up > 0:
		trend = "up"
	case down > 0:
		trend = "down"
	default:
		trend = "unclear"
	}
	if up > 0 && down > 0 || len(changes) >= 3 {
		volatility = "active"
	} else {
		volatility = "quiet"
	}
	if len(changes) == 0 {
		return trend, volatility, "no saved changes in window"
	}
	return trend, volatility, fmt.Sprintf("%d changes over the %s trend window", len(changes), trend)
}

func summarizeSubjectHorizonAbstraction(out memory.SubjectHorizonMemory) string {
	subject := firstTrimmed(out.CanonicalSubject, out.Subject, "subject")
	if len(out.KeyChanges) == 0 {
		return fmt.Sprintf("%s has no saved changes in the %s horizon.", subject, out.Horizon)
	}
	latest := out.KeyChanges[len(out.KeyChanges)-1].ChangeText
	if len(out.DriverClusters) == 0 {
		return fmt.Sprintf("%s %s horizon has %d saved changes; latest: %s.", subject, out.Horizon, len(out.KeyChanges), latest)
	}
	topDriver := out.DriverClusters[0].Subject
	return fmt.Sprintf("%s %s horizon has %d saved changes; latest: %s. Top driver: %s.", subject, out.Horizon, len(out.KeyChanges), latest, topDriver)
}

func subjectHorizonInputHash(out memory.SubjectHorizonMemory) string {
	payload, _ := json.Marshal(struct {
		Horizon string
		Start   string
		End     string
		Changes []memory.SubjectHorizonChange
		Drivers []memory.SubjectHorizonDriver
	}{out.Horizon, out.WindowStart, out.WindowEnd, out.KeyChanges, out.DriverClusters})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func (s *SQLiteStore) upsertSubjectHorizonMemory(ctx context.Context, out memory.SubjectHorizonMemory) error {
	payload, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO subject_horizon_memories(user_id, subject, canonical_subject, horizon, window_start, window_end, refresh_policy, next_refresh_at, input_hash, payload_json, generated_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, canonical_subject, horizon) DO UPDATE SET
		  subject = excluded.subject,
		  window_start = excluded.window_start,
		  window_end = excluded.window_end,
		  refresh_policy = excluded.refresh_policy,
		  next_refresh_at = excluded.next_refresh_at,
		  input_hash = excluded.input_hash,
		  payload_json = excluded.payload_json,
		  generated_at = excluded.generated_at,
		  updated_at = excluded.updated_at`,
		out.UserID,
		out.Subject,
		out.CanonicalSubject,
		out.Horizon,
		out.WindowStart,
		out.WindowEnd,
		out.RefreshPolicy,
		out.NextRefreshAt,
		out.InputHash,
		string(payload),
		out.GeneratedAt,
		out.GeneratedAt,
	)
	return err
}
