package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

const (
	ProjectionDirtyPending        = "pending"
	projectionDirtySubjectWorkers = 4
)

type ProjectionDirtyMark struct {
	ID        int64  `json:"dirty_id,omitempty"`
	UserID    string `json:"user_id"`
	Layer     string `json:"layer"`
	Subject   string `json:"subject,omitempty"`
	Ticker    string `json:"ticker,omitempty"`
	Horizon   string `json:"horizon,omitempty"`
	Reason    string `json:"reason,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
	Status    string `json:"status"`
	DirtyAt   string `json:"dirty_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type ProjectionDirtySweepResult struct {
	UserID    string                      `json:"user_id,omitempty"`
	Limit     int                         `json:"limit"`
	Workers   int                         `json:"workers"`
	Scanned   int                         `json:"scanned"`
	Completed int                         `json:"completed"`
	Failed    int                         `json:"failed"`
	Remaining int                         `json:"remaining"`
	Layers    map[string]int              `json:"layers,omitempty"`
	Errors    []ProjectionDirtySweepError `json:"errors,omitempty"`
}

type ProjectionDirtySweepError struct {
	DirtyID int64  `json:"dirty_id,omitempty"`
	Layer   string `json:"layer"`
	Subject string `json:"subject,omitempty"`
	Horizon string `json:"horizon,omitempty"`
	Error   string `json:"error"`
}

type projectionDirtyExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type projectionDirtyUserState struct {
	mu                sync.Mutex
	eventRefreshed    bool
	paradigmRefreshed bool
	subjectHorizons   map[string]memory.SubjectHorizonMemory
	subjectGraphs     map[string][]graphmodel.ContentSubgraph
	subjectGraphLoads map[string]*projectionDirtyContentGraphLoad
	canonicalSubjects map[string]string
}

type projectionDirtyContentGraphLoad struct {
	done   chan struct{}
	graphs []graphmodel.ContentSubgraph
	err    error
}

type projectionDirtyMarkRunner func(context.Context, ProjectionDirtyMark, *projectionDirtyUserState) error

type projectionDirtyMarkClearer func(context.Context, []ProjectionDirtyMark) error

func (s *SQLiteStore) MarkProjectionDirty(ctx context.Context, mark ProjectionDirtyMark, at time.Time) error {
	return markProjectionDirty(ctx, s.db, mark, at)
}

func markProjectionDirty(ctx context.Context, execer projectionDirtyExecer, mark ProjectionDirtyMark, at time.Time) error {
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	layer := strings.TrimSpace(mark.Layer)
	if layer == "" {
		return fmt.Errorf("projection layer is required")
	}
	at = normalizeRecordedTime(at)
	now := currentSQLiteTimestamp()
	_, err = execer.ExecContext(ctx, `INSERT INTO projection_dirty_marks(user_id, layer, subject, ticker, horizon, reason, source_ref, status, dirty_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, layer, subject, ticker, horizon) DO UPDATE SET
			reason = excluded.reason,
			source_ref = excluded.source_ref,
			status = excluded.status,
			dirty_at = excluded.dirty_at,
			updated_at = excluded.updated_at`,
		userID,
		layer,
		normalizeDirtyDimension(mark.Subject),
		normalizeDirtyDimension(mark.Ticker),
		normalizeDirtyDimension(mark.Horizon),
		strings.TrimSpace(mark.Reason),
		strings.TrimSpace(mark.SourceRef),
		ProjectionDirtyPending,
		at.UTC().Format(time.RFC3339Nano),
		now,
	)
	return err
}

func (s *SQLiteStore) ListProjectionDirtyMarks(ctx context.Context, userID string, limit int) ([]ProjectionDirtyMark, error) {
	userID = strings.TrimSpace(userID)
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT dirty_id, user_id, layer, subject, ticker, horizon, reason, source_ref, status, dirty_at, updated_at
		FROM projection_dirty_marks WHERE status = ?`
	args := []any{ProjectionDirtyPending}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY dirty_at ASC, dirty_id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProjectionDirtyMark, 0)
	for rows.Next() {
		var mark ProjectionDirtyMark
		if err := rows.Scan(&mark.ID, &mark.UserID, &mark.Layer, &mark.Subject, &mark.Ticker, &mark.Horizon, &mark.Reason, &mark.SourceRef, &mark.Status, &mark.DirtyAt, &mark.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, mark)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) HasProjectionDirtyMark(ctx context.Context, userID, layer, subject, horizon string) (bool, error) {
	userID = strings.TrimSpace(userID)
	layer = strings.TrimSpace(layer)
	if userID == "" || layer == "" {
		return false, nil
	}
	query := `SELECT 1 FROM projection_dirty_marks WHERE status = ? AND user_id = ? AND layer = ?`
	args := []any{ProjectionDirtyPending, userID, layer}
	if strings.TrimSpace(subject) != "" {
		query += ` AND subject = ?`
		args = append(args, normalizeDirtyDimension(subject))
	}
	if strings.TrimSpace(horizon) != "" {
		query += ` AND horizon = ?`
		args = append(args, normalizeDirtyDimension(horizon))
	}
	query += ` LIMIT 1`
	var one int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) RunProjectionDirtySweep(ctx context.Context, userID string, limit int, now time.Time) (ProjectionDirtySweepResult, error) {
	return s.RunProjectionDirtySweepWithWorkers(ctx, userID, limit, 1, now)
}

func (s *SQLiteStore) RunProjectionDirtySweepWithWorkers(ctx context.Context, userID string, limit int, workers int, now time.Time) (ProjectionDirtySweepResult, error) {
	userID = strings.TrimSpace(userID)
	if limit <= 0 {
		limit = 100
	}
	if workers <= 0 {
		workers = 1
	}
	now = normalizeRecordedTime(now)
	result := ProjectionDirtySweepResult{
		UserID:  userID,
		Limit:   limit,
		Workers: workers,
	}
	marks, err := s.ListProjectionDirtyMarks(ctx, userID, limit)
	if err != nil {
		return result, err
	}
	result = mergeProjectionDirtySweepResult(result, runProjectionDirtyMarkGroups(ctx, marks, workers, func(ctx context.Context, mark ProjectionDirtyMark, state *projectionDirtyUserState) error {
		return s.runProjectionDirtyMark(ctx, mark, now, state)
	}, s.ClearProjectionDirtyMarks))
	remaining, err := s.countProjectionDirtyMarks(ctx, userID)
	if err != nil {
		return result, err
	}
	result.Remaining = remaining
	if result.Completed == 0 {
		result.Layers = nil
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("projection dirty sweep failed for %d mark(s)", result.Failed)
	}
	return result, nil
}

func runProjectionDirtyMarkGroups(ctx context.Context, marks []ProjectionDirtyMark, workers int, runner projectionDirtyMarkRunner, clearer projectionDirtyMarkClearer) ProjectionDirtySweepResult {
	if workers <= 0 {
		workers = 1
	}
	result := ProjectionDirtySweepResult{Workers: workers, Scanned: len(marks), Layers: map[string]int{}}
	groups := groupProjectionDirtyMarksByUser(marks)
	if len(groups) == 0 {
		result.Layers = nil
		return result
	}
	if workers > len(groups) {
		workers = len(groups)
	}
	jobs := make(chan []ProjectionDirtyMark)
	results := make(chan ProjectionDirtySweepResult, len(groups))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobs {
				results <- runProjectionDirtyMarkGroup(ctx, group, runner, clearer)
			}
		}()
	}
	for _, group := range groups {
		jobs <- group
	}
	close(jobs)
	wg.Wait()
	close(results)
	for item := range results {
		result = mergeProjectionDirtySweepResult(result, item)
	}
	if result.Completed == 0 {
		result.Layers = nil
	}
	return result
}

func groupProjectionDirtyMarksByUser(marks []ProjectionDirtyMark) [][]ProjectionDirtyMark {
	order := make([]string, 0)
	byUser := map[string][]ProjectionDirtyMark{}
	for _, mark := range marks {
		userID := strings.TrimSpace(mark.UserID)
		if _, ok := byUser[userID]; !ok {
			order = append(order, userID)
		}
		byUser[userID] = append(byUser[userID], mark)
	}
	out := make([][]ProjectionDirtyMark, 0, len(order))
	for _, userID := range order {
		out = append(out, byUser[userID])
	}
	return out
}

func runProjectionDirtyMarkGroup(ctx context.Context, marks []ProjectionDirtyMark, runner projectionDirtyMarkRunner, clearer projectionDirtyMarkClearer) ProjectionDirtySweepResult {
	result := ProjectionDirtySweepResult{Layers: map[string]int{}}
	state := &projectionDirtyUserState{}
	successful := make([]ProjectionDirtyMark, 0, len(marks))
	for _, phase := range projectionDirtyMarkGroupPhases(orderProjectionDirtyMarksForSweep(marks)) {
		if phase.concurrent {
			outcomes := runProjectionDirtyMarksConcurrently(ctx, phase.marks, projectionDirtySubjectWorkers, runner, state)
			for _, outcome := range outcomes {
				recordProjectionDirtyRunOutcome(&result, &successful, outcome)
			}
			continue
		}
		for _, mark := range phase.marks {
			recordProjectionDirtyRunOutcome(&result, &successful, runProjectionDirtyMarkWithState(ctx, mark, runner, state))
		}
	}
	if len(successful) > 0 {
		if err := clearer(ctx, successful); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, projectionDirtySweepError(successful[0], err))
			successful = nil
		}
	}
	for _, mark := range successful {
		result.Completed++
		result.Layers[strings.TrimSpace(mark.Layer)]++
	}
	if result.Completed == 0 {
		result.Layers = nil
	}
	return result
}

type projectionDirtyMarkPhase struct {
	marks      []ProjectionDirtyMark
	concurrent bool
}

type projectionDirtyRunOutcome struct {
	mark ProjectionDirtyMark
	err  error
}

func projectionDirtyMarkGroupPhases(marks []ProjectionDirtyMark) []projectionDirtyMarkPhase {
	var base []ProjectionDirtyMark
	var global []ProjectionDirtyMark
	var horizons []ProjectionDirtyMark
	var experience []ProjectionDirtyMark
	var other []ProjectionDirtyMark
	for _, mark := range marks {
		switch strings.TrimSpace(mark.Layer) {
		case "event", "paradigm":
			base = append(base, mark)
		case "global-v2":
			global = append(global, mark)
		case "subject-horizon":
			horizons = append(horizons, mark)
		case "subject-experience":
			experience = append(experience, mark)
		default:
			other = append(other, mark)
		}
	}
	phases := make([]projectionDirtyMarkPhase, 0, 6)
	appendPhase := func(items []ProjectionDirtyMark, concurrent bool) {
		if len(items) > 0 {
			phases = append(phases, projectionDirtyMarkPhase{marks: items, concurrent: concurrent})
		}
	}
	appendPhase(base, false)
	appendPhase(global, false)
	appendPhase(horizons, true)
	appendPhase(experience, false)
	appendPhase(other, false)
	return phases
}

func runProjectionDirtyMarksConcurrently(ctx context.Context, marks []ProjectionDirtyMark, workers int, runner projectionDirtyMarkRunner, state *projectionDirtyUserState) []projectionDirtyRunOutcome {
	if len(marks) == 0 {
		return nil
	}
	if workers <= 0 || workers > len(marks) {
		workers = len(marks)
	}
	jobs := make(chan ProjectionDirtyMark)
	outcomes := make(chan projectionDirtyRunOutcome, len(marks))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mark := range jobs {
				outcomes <- runProjectionDirtyMarkWithState(ctx, mark, runner, state)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, mark := range marks {
			jobs <- mark
		}
	}()
	wg.Wait()
	close(outcomes)
	out := make([]projectionDirtyRunOutcome, 0, len(marks))
	for outcome := range outcomes {
		out = append(out, outcome)
	}
	return out
}

func runProjectionDirtyMarkWithState(ctx context.Context, mark ProjectionDirtyMark, runner projectionDirtyMarkRunner, state *projectionDirtyUserState) projectionDirtyRunOutcome {
	if err := ctx.Err(); err != nil {
		return projectionDirtyRunOutcome{mark: mark, err: err}
	}
	return projectionDirtyRunOutcome{mark: mark, err: runner(ctx, mark, state)}
}

func recordProjectionDirtyRunOutcome(result *ProjectionDirtySweepResult, successful *[]ProjectionDirtyMark, outcome projectionDirtyRunOutcome) {
	if outcome.err != nil {
		result.Failed++
		result.Errors = append(result.Errors, projectionDirtySweepError(outcome.mark, outcome.err))
		return
	}
	*successful = append(*successful, outcome.mark)
}

func mergeProjectionDirtySweepResult(base ProjectionDirtySweepResult, add ProjectionDirtySweepResult) ProjectionDirtySweepResult {
	base.Scanned += add.Scanned
	base.Completed += add.Completed
	base.Failed += add.Failed
	if len(add.Errors) > 0 {
		base.Errors = append(base.Errors, add.Errors...)
	}
	if len(add.Layers) > 0 {
		if base.Layers == nil {
			base.Layers = map[string]int{}
		}
		for layer, count := range add.Layers {
			base.Layers[layer] += count
		}
	}
	return base
}

func orderProjectionDirtyMarksForSweep(marks []ProjectionDirtyMark) []ProjectionDirtyMark {
	out := append([]ProjectionDirtyMark(nil), marks...)
	sort.SliceStable(out, func(i, j int) bool {
		return projectionDirtyLayerPriority(out[i].Layer) < projectionDirtyLayerPriority(out[j].Layer)
	})
	return out
}

func projectionDirtyLayerPriority(layer string) int {
	switch strings.TrimSpace(layer) {
	case "event", "paradigm":
		return 0
	case "global-v2":
		return 1
	default:
		return 2
	}
}

func (s *SQLiteStore) runProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark, now time.Time, state *projectionDirtyUserState) error {
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(mark.Layer) {
	case "event":
		_, err = s.RunEventGraphProjection(ctx, userID, now)
		if err == nil && state != nil {
			state.eventRefreshed = true
		}
	case "paradigm":
		_, err = s.RunParadigmProjection(ctx, userID, now)
		if err == nil && state != nil {
			state.paradigmRefreshed = true
		}
	case "global-v2":
		refreshProjections := state == nil || !state.eventRefreshed || !state.paradigmRefreshed
		_, err = s.runGlobalMemoryOrganizationV2(ctx, userID, now, refreshProjections)
		if err == nil && refreshProjections && state != nil {
			state.eventRefreshed = true
			state.paradigmRefreshed = true
		}
	case "subject-timeline":
		if strings.TrimSpace(mark.Subject) == "" {
			return fmt.Errorf("subject-timeline mark requires subject")
		}
		return nil
	case "subject-horizon":
		if strings.TrimSpace(mark.Subject) == "" || strings.TrimSpace(mark.Horizon) == "" {
			return fmt.Errorf("subject-horizon mark requires subject and horizon")
		}
		var item memory.SubjectHorizonMemory
		var graphs []graphmodel.ContentSubgraph
		hasGraphInputs := false
		if state != nil {
			graphs, err = state.memoryContentGraphsBySubject(ctx, userID, mark.Subject, s.ListMemoryContentGraphsBySubject)
			if err != nil {
				return err
			}
			hasGraphInputs = true
		}
		var resolve projectionSubjectResolver
		if state != nil {
			resolve = func(ctx context.Context, node graphmodel.GraphNode) (string, error) {
				return state.canonicalGraphNodeSubject(ctx, node, s.resolveCanonicalGraphNodeSubject)
			}
		}
		item, err = s.getSubjectHorizonMemory(ctx, userID, mark.Subject, mark.Horizon, now, true, graphs, hasGraphInputs, resolve)
		if err == nil && state != nil {
			state.storeSubjectHorizon(item)
		}
	case "subject-experience":
		if strings.TrimSpace(mark.Subject) == "" {
			return fmt.Errorf("subject-experience mark requires subject")
		}
		preloaded := map[string]memory.SubjectHorizonMemory(nil)
		if state != nil {
			preloaded = state.preloadedSubjectHorizons(mark.Subject, defaultSubjectExperienceHorizons)
		}
		_, err = s.getSubjectExperienceMemoryWithHorizonInputs(ctx, userID, mark.Subject, defaultSubjectExperienceHorizons, now, false, preloaded)
	default:
		err = fmt.Errorf("unsupported projection layer %q", mark.Layer)
	}
	return err
}

func (state *projectionDirtyUserState) memoryContentGraphsBySubject(ctx context.Context, userID, subject string, load func(context.Context, string, string) ([]graphmodel.ContentSubgraph, error)) ([]graphmodel.ContentSubgraph, error) {
	key := strings.TrimSpace(userID) + "\x00" + normalizeDirtyDimension(subject)
	return state.memoryContentGraphsForKey(ctx, key, func(ctx context.Context) ([]graphmodel.ContentSubgraph, error) {
		return load(ctx, userID, subject)
	})
}

func (state *projectionDirtyUserState) memoryContentGraphsForKey(ctx context.Context, key string, load func(context.Context) ([]graphmodel.ContentSubgraph, error)) ([]graphmodel.ContentSubgraph, error) {
	if state == nil {
		return load(ctx)
	}
	state.mu.Lock()
	if state.subjectGraphs != nil {
		if graphs, ok := state.subjectGraphs[key]; ok {
			state.mu.Unlock()
			return graphs, nil
		}
	}
	if state.subjectGraphLoads == nil {
		state.subjectGraphLoads = map[string]*projectionDirtyContentGraphLoad{}
	}
	if inFlight := state.subjectGraphLoads[key]; inFlight != nil {
		done := inFlight.done
		state.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		state.mu.Lock()
		graphs, err := inFlight.graphs, inFlight.err
		state.mu.Unlock()
		return graphs, err
	}
	inFlight := &projectionDirtyContentGraphLoad{done: make(chan struct{})}
	state.subjectGraphLoads[key] = inFlight
	state.mu.Unlock()

	graphs, err := load(ctx)

	state.mu.Lock()
	inFlight.graphs = graphs
	inFlight.err = err
	if err == nil {
		if state.subjectGraphs == nil {
			state.subjectGraphs = map[string][]graphmodel.ContentSubgraph{}
		}
		state.subjectGraphs[key] = graphs
	}
	delete(state.subjectGraphLoads, key)
	close(inFlight.done)
	state.mu.Unlock()
	return graphs, err
}

func (state *projectionDirtyUserState) canonicalGraphNodeSubject(ctx context.Context, node graphmodel.GraphNode, resolve func(context.Context, graphmodel.GraphNode, map[string]string) (string, error)) (string, error) {
	if state == nil {
		return resolve(ctx, node, nil)
	}
	key := normalizeDirtyDimension(firstTrimmed(node.SubjectCanonical, node.SubjectText))
	if key == "" {
		return "", nil
	}
	state.mu.Lock()
	if state.canonicalSubjects != nil {
		if subject, ok := state.canonicalSubjects[key]; ok {
			state.mu.Unlock()
			return subject, nil
		}
	}
	state.mu.Unlock()

	subject, err := resolve(ctx, node, nil)
	if err != nil {
		return "", err
	}
	subject = strings.TrimSpace(subject)

	state.mu.Lock()
	if state.canonicalSubjects == nil {
		state.canonicalSubjects = map[string]string{}
	}
	state.canonicalSubjects[key] = subject
	state.mu.Unlock()
	return subject, nil
}

func (state *projectionDirtyUserState) storeSubjectHorizon(item memory.SubjectHorizonMemory) {
	if state == nil || strings.TrimSpace(item.Horizon) == "" {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.subjectHorizons == nil {
		state.subjectHorizons = map[string]memory.SubjectHorizonMemory{}
	}
	state.subjectHorizons[subjectHorizonStateKey(item.CanonicalSubject, item.Horizon)] = item
	state.subjectHorizons[subjectHorizonStateKey(item.Subject, item.Horizon)] = item
}

func (state *projectionDirtyUserState) preloadedSubjectHorizons(subject string, horizons []string) map[string]memory.SubjectHorizonMemory {
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.subjectHorizons) == 0 {
		return nil
	}
	out := map[string]memory.SubjectHorizonMemory{}
	for _, horizon := range horizons {
		key := subjectHorizonStateKey(subject, horizon)
		if item, ok := state.subjectHorizons[key]; ok {
			out[strings.TrimSpace(horizon)] = item
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func subjectHorizonStateKey(subject, horizon string) string {
	return normalizeDirtyDimension(subject) + "\x00" + strings.TrimSpace(horizon)
}

func (s *SQLiteStore) countProjectionDirtyMarks(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM projection_dirty_marks WHERE status = ?`
	args := []any{ProjectionDirtyPending}
	if strings.TrimSpace(userID) != "" {
		query += ` AND user_id = ?`
		args = append(args, strings.TrimSpace(userID))
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func projectionDirtySweepError(mark ProjectionDirtyMark, err error) ProjectionDirtySweepError {
	return ProjectionDirtySweepError{
		DirtyID: mark.ID,
		Layer:   strings.TrimSpace(mark.Layer),
		Subject: strings.TrimSpace(mark.Subject),
		Horizon: strings.TrimSpace(mark.Horizon),
		Error:   err.Error(),
	}
}

func (s *SQLiteStore) ClearProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark) error {
	return s.ClearProjectionDirtyMarks(ctx, []ProjectionDirtyMark{mark})
}

func (s *SQLiteStore) ClearProjectionDirtyMarks(ctx context.Context, marks []ProjectionDirtyMark) error {
	if len(marks) == 0 {
		return nil
	}
	if len(marks) == 1 {
		return s.clearProjectionDirtyMark(ctx, marks[0])
	}
	ids := make([]int64, 0, len(marks))
	for _, mark := range marks {
		if mark.ID <= 0 {
			return s.clearProjectionDirtyMarksIndividually(ctx, marks)
		}
		ids = append(ids, mark.ID)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM projection_dirty_marks WHERE dirty_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(ids)) {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteStore) clearProjectionDirtyMarksIndividually(ctx context.Context, marks []ProjectionDirtyMark) error {
	for _, mark := range marks {
		if err := s.clearProjectionDirtyMark(ctx, mark); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) clearProjectionDirtyMark(ctx context.Context, mark ProjectionDirtyMark) error {
	if mark.ID > 0 {
		result, err := s.db.ExecContext(ctx, `DELETE FROM projection_dirty_marks WHERE dirty_id = ?`, mark.ID)
		if err != nil {
			return err
		}
		return ensureDirtyMarkDeleted(result)
	}
	userID, err := normalizeRequiredUserID(mark.UserID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(mark.Layer) == "" {
		return fmt.Errorf("projection layer is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM projection_dirty_marks
		WHERE user_id = ? AND layer = ? AND subject = ? AND ticker = ? AND horizon = ?`,
		userID,
		strings.TrimSpace(mark.Layer),
		normalizeDirtyDimension(mark.Subject),
		normalizeDirtyDimension(mark.Ticker),
		normalizeDirtyDimension(mark.Horizon),
	)
	if err != nil {
		return err
	}
	return ensureDirtyMarkDeleted(result)
}

func ensureDirtyMarkDeleted(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeDirtyDimension(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
